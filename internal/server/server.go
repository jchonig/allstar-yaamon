package server

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"allstar-yaamon/internal/ami"
	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
	tlsserver "allstar-yaamon/internal/tls"
)

type Server struct {
	cfg      *config.Config
	webFS    embed.FS
	db       *db.DB
	sessions *auth.Manager
	amiMgr   *ami.Manager
	tmpls    map[string]*template.Template
}

func New(cfg *config.Config, database *db.DB, webFS embed.FS) (*Server, error) {
	s := &Server{cfg: cfg, db: database, webFS: webFS}
	if err := s.initSessions(); err != nil {
		return nil, fmt.Errorf("session manager: %w", err)
	}
	if err := s.parseTemplates(); err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	if err := s.initAMI(); err != nil {
		return nil, fmt.Errorf("AMI manager: %w", err)
	}
	return s, nil
}

func (s *Server) initAMI() error {
	nodes, err := s.db.ListNodes(context.Background())
	if err != nil {
		return err
	}
	s.amiMgr = ami.NewManager()
	s.amiMgr.LoadNodes(nodes)
	return nil
}

func (s *Server) initSessions() error {
	ctx := context.Background()
	secret, err := s.db.GetConfig(ctx, "session_secret")
	if err != nil {
		return err
	}
	if secret == "" {
		secret, err = auth.GenerateSecret()
		if err != nil {
			return err
		}
		if err := s.db.SetConfig(ctx, "session_secret", secret); err != nil {
			return err
		}
	}
	raw, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return fmt.Errorf("invalid session secret: %w", err)
	}
	s.sessions = auth.NewManager(raw, s.cfg.TLS.Mode != "disabled")
	return nil
}

func (s *Server) parseTemplates() error {
	pages := []string{"login", "dashboard", "setup", "nodes"}
	s.tmpls = make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.ParseFS(s.webFS,
			"web/templates/base.html",
			"web/templates/"+page+".html",
		)
		if err != nil {
			return fmt.Errorf("page %s: %w", page, err)
		}
		s.tmpls[page] = t
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	t, ok := s.tmpls[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		slog.Error("template render", "page", page, "err", err)
	}
}

func (s *Server) Run() error {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(s.setupGuard) // redirect to /setup before auth fires when no users exist
	r.Use(s.sessions.Middleware)

	// Static files (served before auth middleware so login page CSS loads)
	staticFS, err := fs.Sub(s.webFS, "web/static")
	if err != nil {
		return fmt.Errorf("web/static embed: %w", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Public routes
	r.Get("/health", s.handleHealth)
	r.Get("/setup", s.handleSetupGet)
	r.Post("/setup", s.handleSetupPost)
	r.Get("/login", s.handleLoginGet)
	r.Post("/login", s.handleLoginPost)
	r.Get("/logout", s.handleLogout)

	// Protected routes — readonly+
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.RequirePermission(db.PermReadOnly))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
		})
		r.Get("/dashboard", s.handleDashboard)
		r.Get("/dashboard/{nodeID}", s.handleDashboard)
		r.Get("/api/nodes", s.handleAPIListNodes)
	})

	// Admin routes — admin+
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.RequirePermission(db.PermAdmin))
		r.Get("/admin/nodes", s.handleNodesPage)
		r.Post("/api/nodes", s.handleAPICreateNode)
		r.Put("/api/nodes/{id}", s.handleAPIUpdateNode)
		r.Delete("/api/nodes/{id}", s.handleAPIDeleteNode)
		r.Post("/api/nodes/{id}/test", s.handleAPITestNode)
	})

	return s.listenAndServe(r)
}

func (s *Server) listenAndServe(handler http.Handler) error {
	tlsCfg, err := tlsserver.NewTLSConfig(&s.cfg.TLS)
	if err != nil {
		return fmt.Errorf("TLS config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var mainServer *http.Server

	if tlsCfg != nil {
		httpsAddr := fmt.Sprintf(":%d", s.cfg.Server.HTTPSPort)
		mainServer = &http.Server{
			Addr:         httpsAddr,
			Handler:      handler,
			TLSConfig:    tlsCfg,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}
		if s.cfg.Server.RedirectHTTP {
			httpAddr := fmt.Sprintf(":%d", s.cfg.Server.HTTPPort)
			redirect := &http.Server{
				Addr:        httpAddr,
				Handler:     tlsserver.RedirectHandler(s.cfg.Server.HTTPSPort),
				ReadTimeout: 10 * time.Second,
			}
			go func() {
				slog.Info("HTTP redirect listener", "addr", httpAddr)
				if err := redirect.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("HTTP redirect server", "err", err)
				}
			}()
		}
		ln, err := net.Listen("tcp", httpsAddr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", httpsAddr, err)
		}
		slog.Info("HTTPS server listening", "addr", httpsAddr, "tls_mode", s.cfg.TLS.Mode)
		go func() {
			if err := mainServer.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
				slog.Error("HTTPS server", "err", err)
			}
		}()
	} else {
		httpAddr := fmt.Sprintf(":%d", s.cfg.Server.HTTPPort)
		mainServer = &http.Server{
			Addr:         httpAddr,
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}
		slog.Info("HTTP server listening", "addr", httpAddr)
		go func() {
			if err := mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("HTTP server", "err", err)
			}
		}()
	}

	<-ctx.Done()
	slog.Info("shutting down")
	s.amiMgr.Shutdown()
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return mainServer.Shutdown(shutCtx)
}
