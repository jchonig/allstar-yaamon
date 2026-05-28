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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"allstar-yaamon/internal/ami"
	"allstar-yaamon/internal/aslstats"
	"allstar-yaamon/internal/astdb"
	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
	"allstar-yaamon/internal/sse"
	tlsserver "allstar-yaamon/internal/tls"
)

// nodeDBer is the subset of astdb.DB used by the server so tests can stub it.
type nodeDBer interface {
	Lookup(nodeNumber string) (astdb.Node, bool)
	Start(ctx context.Context, interval time.Duration)
}

type Server struct {
	cfg          *config.Config
	webFS        embed.FS
	db           *db.DB
	sessions     *auth.Manager
	amiMgr       *ami.Manager
	fetcher      *aslstats.Fetcher
	nodeDB       nodeDBer
	statsCache   *statsCache
	linksCache   *linksCache
	sseBroker    *sse.Broker
	tmpls        map[string]*template.Template
	loginLimiter *loginLimiter

	// adaptive stats-fetch mode: switches between individual and bulk endpoint
	// based on how many stale node numbers are pending (see fetchAdaptive).
	adaptiveMu sync.Mutex
	inBulkMode bool
	lastBulkAt time.Time
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
	s.fetcher = aslstats.New("")
	s.nodeDB = astdb.New(cfg.AstDB.Path, cfg.AstDB.Update)
	s.statsCache = newStatsCache()
	s.linksCache = newLinksCache()
	s.sseBroker = sse.NewBroker()
	s.loginLimiter = newLoginLimiter()
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
	pages := []string{"login", "dashboard", "setup", "nodes", "users", "backup", "favorites", "graph"}
	s.tmpls = make(map[string]*template.Template, len(pages))
	ui := s.cfg.UI
	funcMap := template.FuncMap{
		"siteFooterText":           func() string { return ui.FooterText },
		"siteFooterURL":            func() string { return ui.FooterURL },
		"siteFooterAttribution":    func() string { return ui.FooterAttribution },
		"siteFooterAttributionURL": func() string { return ui.FooterAttributionURL },
		"isLocalAvatarURL": func(url string) bool {
			return strings.HasPrefix(url, "/api/users/")
		},
	}
	for _, page := range pages {
		t, err := template.New("root").Funcs(funcMap).ParseFS(s.webFS,
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
	r.Use(csrfMiddleware)
	r.Use(s.setupGuard) // redirect to /setup before auth fires when no users exist
	r.Use(s.sessions.Middleware)
	r.Use(s.validateSessionUser) // reject sessions for deleted accounts

	// Favicon routes — check for custom favicon in DB before falling back to embedded.
	r.Get("/favicon.ico", s.serveFavicon("favicon.ico", "image/x-icon"))
	r.Get("/favicon.png", s.serveFavicon("favicon-256.png", "image/png"))

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
		r.Get("/dashboard/overview", s.handleDashboardOverview)
		r.Get("/dashboard/{nodeID}", s.handleDashboard)
		r.Get("/sse/{nodeID}", s.handleSSE)
		r.Get("/api/nodes", s.handleAPIListNodes)
		r.Get("/api/nodes/{id}/favorites", s.handleAPIListFavorites)
		r.Get("/api/nodes/{id}/stats", s.handleAPINodeStats)
		r.Get("/api/nodes/{id}/connections/{nodeNumber}", s.handleAPIConnections)
		r.Get("/graph/{nodeNumber}", s.handleGraphPage)
		r.Put("/api/profile", s.handleAPIUpdateProfile)
		r.Post("/api/profile/avatar", s.handleAPIUploadAvatar)
		r.Delete("/api/profile/avatar", s.handleAPIDeleteAvatar)
		r.Get("/api/users/{id}/avatar", s.handleAPIGetAvatar)
	})

	// Readwrite+ routes — can connect/disconnect and manage favorites
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.RequirePermission(db.PermReadWrite))
		r.Get("/settings/favorites", s.handleFavoritesPage)
		r.Get("/settings/favorites/{nodeID}", s.handleFavoritesPage)
		r.Post("/api/nodes/{id}/connect", s.handleConnect)
		r.Post("/api/nodes/{id}/disconnect", s.handleDisconnect)
		r.Post("/api/nodes/{id}/favorites", s.handleAPICreateFavorite)
		r.Put("/api/nodes/{id}/favorites/{fid}", s.handleAPIUpdateFavorite)
		r.Put("/api/nodes/{id}/favorites/reorder", s.handleAPIReorderFavorites)
		r.Post("/api/nodes/{id}/favorites/copy", s.handleAPICopyFavorites)
		r.Delete("/api/nodes/{id}/favorites/{fid}", s.handleAPIDeleteFavorite)
	})

	// Admin routes — admin+
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.RequirePermission(db.PermAdmin))
		r.Post("/api/admin/favicon", s.handleAPIUploadFavicon)
		r.Delete("/api/admin/favicon", s.handleAPIDeleteFavicon)
		r.Post("/api/admin/import/allmon3/preview", s.handleAPIImportAllmon3Preview)
		r.Post("/api/admin/import/allmon3", s.handleAPIImportAllmon3)
		r.Get("/admin/nodes", s.handleNodesPage)
		r.Post("/api/nodes", s.handleAPICreateNode)
		r.Put("/api/nodes/{id}", s.handleAPIUpdateNode)
		r.Delete("/api/nodes/{id}", s.handleAPIDeleteNode)
		r.Post("/api/nodes/{id}/test", s.handleAPITestNode)
		r.Get("/api/nodes/{id}/secret", s.handleAPINodeSecret)
		r.Get("/admin/users", s.handleUsersPage)
		r.Get("/api/users", s.handleAPIListUsers)
		r.Post("/api/users", s.handleAPICreateUser)
		r.Put("/api/users/{id}", s.handleAPIUpdateUser)
		r.Delete("/api/users/{id}", s.handleAPIDeleteUser)
		r.Get("/admin/backup", s.handleBackupPage)
		r.Post("/api/backup", s.handleAPIBackup)
		r.Post("/api/backup/inspect", s.handleAPIBackupInspect)
		r.Post("/api/backup/restore", s.handleAPIBackupRestore)
	})

	// Readwrite+ favorites export/import
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.RequirePermission(db.PermReadOnly))
		r.Get("/api/nodes/{id}/favorites/export", s.handleAPIFavoritesExport)
	})
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.RequirePermission(db.PermReadWrite))
		r.Post("/api/nodes/{id}/favorites/import/preview", s.handleAPIFavoritesImportPreview)
		r.Post("/api/nodes/{id}/favorites/import", s.handleAPIFavoritesImport)
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

	s.startStatsPoller(ctx)
	s.startLinksPoller(ctx)
	s.startAMIEventListener(ctx)
	s.nodeDB.Start(ctx, 1*time.Hour)

	var mainServer *http.Server

	baseCtx := func(_ net.Listener) context.Context { return ctx }

	if tlsCfg != nil {
		httpsAddr := fmt.Sprintf(":%d", s.cfg.Server.HTTPSPort)
		mainServer = &http.Server{
			Addr:        httpsAddr,
			Handler:     hstsMiddleware(handler),
			TLSConfig:   tlsCfg,
			BaseContext: baseCtx,
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
			Addr:        httpAddr,
			Handler:     handler,
			BaseContext: baseCtx,
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
