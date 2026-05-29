// Package web wires HTTP routes, templates, and static assets together.
package web

import (
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"net/http"

	"mederos-crm/internal/auth"
	"mederos-crm/internal/config"
	"mederos-crm/internal/files"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds shared dependencies for handlers.
type Server struct {
	DB      *sql.DB
	Cfg     config.Config
	Auth    *auth.Manager
	rnd     *renderer
	browser *files.Browser // nil when case-files root is not configured
	logger  *log.Logger
}

// NewServer constructs the Server, parses templates, and opens the case-files
// browser if configured.
func NewServer(db *sql.DB, cfg config.Config, logger *log.Logger) (*Server, error) {
	tplSub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	rnd, err := newRenderer(tplSub)
	if err != nil {
		return nil, err
	}

	s := &Server{
		DB:     db,
		Cfg:    cfg,
		Auth:   auth.NewManager(db, cfg.SessionTTL(), cfg.TLSEnabled()),
		rnd:    rnd,
		logger: logger,
	}

	if cfg.FileBrowsingEnabled() {
		b, err := files.Open(cfg.CaseFilesRoot)
		if err != nil {
			return nil, err
		}
		s.browser = b
	}
	return s, nil
}

// Handler returns the fully-wired HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /setup", s.handleSetupForm)
	mux.HandleFunc("POST /setup", s.handleSetupSubmit)

	// Static assets (embedded).
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/",
		http.FileServer(http.FS(staticSub))))

	// Authenticated routes.
	authed := http.NewServeMux()
	authed.HandleFunc("GET /{$}", s.handleIndex) // exact root match only
	authed.HandleFunc("GET /clients", s.handleClientsList)
	authed.HandleFunc("GET /clients/new", s.handleClientNewForm)
	authed.HandleFunc("POST /clients", s.handleClientCreate)
	authed.HandleFunc("GET /clients/{id}", s.handleClientDetail)
	authed.HandleFunc("GET /clients/{id}/edit", s.handleClientEditForm)
	authed.HandleFunc("POST /clients/{id}", s.handleClientUpdate)
	authed.HandleFunc("POST /clients/{id}/delete", s.handleClientDelete)
	authed.HandleFunc("GET /clients/{id}/files", s.handleClientFiles)
	authed.HandleFunc("GET /clients/{id}/files/download", s.handleClientFileDownload)

	authed.HandleFunc("GET /change-password", s.handleChangePasswordForm)
	authed.HandleFunc("POST /change-password", s.handleChangePasswordSubmit)

	// Admin routes (admin role enforced per-handler group).
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /admin/users", s.handleAdminUsers)
	adminMux.HandleFunc("POST /admin/users", s.handleAdminCreateUser)
	adminMux.HandleFunc("POST /admin/users/{id}/reset-password", s.handleAdminResetPassword)
	adminMux.HandleFunc("POST /admin/users/{id}/deactivate", s.handleAdminDeactivate)
	adminMux.HandleFunc("POST /admin/users/{id}/activate", s.handleAdminActivate)
	adminMux.HandleFunc("GET /admin/audit", s.handleAdminAudit)
	authed.Handle("/admin/", s.Auth.RequireAdmin(adminMux))

	mux.Handle("/", s.Auth.RequireAuth(authed))
	return securityHeaders(mux)
}

// securityHeaders adds a few conservative response headers app-wide.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// logf logs through the server logger if present.
func (s *Server) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}
