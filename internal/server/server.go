package server

import (
	"context"
	"embed"
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

	"allstar-yaamon/internal/config"
	tlsserver "allstar-yaamon/internal/tls"
)

type Server struct {
	cfg       *config.Config
	webFS     embed.FS
	templates map[string]*template.Template
}

func New(cfg *config.Config, webFS embed.FS) (*Server, error) {
	s := &Server{cfg: cfg, webFS: webFS}
	if err := s.parseTemplates(); err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return s, nil
}

// parseTemplates builds a per-page template set (base + page) so that each
// page's {{define "content"}} overrides the base {{block}} without colliding.
func (s *Server) parseTemplates() error {
	pages := []string{"login", "dashboard"}
	s.templates = make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.ParseFS(s.webFS,
			"web/templates/base.html",
			"web/templates/"+page+".html",
		)
		if err != nil {
			return fmt.Errorf("page %s: %w", page, err)
		}
		s.templates[page] = t
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	t, ok := s.templates[page]
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

	staticFS, err := fs.Sub(s.webFS, "web/static")
	if err != nil {
		return fmt.Errorf("web/static embed: %w", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})
	r.Get("/login", s.handleLoginGet)
	r.Post("/login", s.handleLoginPost)
	r.Get("/logout", s.handleLogout)
	r.Get("/dashboard", s.handleDashboard)
	r.Get("/dashboard/{nodeID}", s.handleDashboard)
	r.Get("/health", s.handleHealth)

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
			redirectServer := &http.Server{
				Addr:         httpAddr,
				Handler:      tlsserver.RedirectHandler(s.cfg.Server.HTTPSPort),
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
			}
			go func() {
				slog.Info("HTTP redirect listener", "addr", httpAddr)
				if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return mainServer.Shutdown(shutCtx)
}
