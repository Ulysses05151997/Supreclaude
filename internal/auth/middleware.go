package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"mederos-crm/internal/models"
)

type ctxKey int

const userCtxKey ctxKey = 0

const (
	sessionCookie = "crm_session"
	csrfCookie    = "crm_csrf"
	csrfHeader    = "X-CSRF-Token"
	csrfField     = "csrf_token"
)

// Manager wires sessions to the database and cookie settings.
type Manager struct {
	DB         *sql.DB
	TTL        time.Duration
	SecureCook bool // set Secure flag on cookies (only when serving HTTPS)
}

// NewManager builds a session manager.
func NewManager(db *sql.DB, ttl time.Duration, secure bool) *Manager {
	return &Manager{DB: db, TTL: ttl, SecureCook: secure}
}

// Login creates a session for the user, sets the session and CSRF cookies, and
// returns the new session id.
func (m *Manager) Login(w http.ResponseWriter, r *http.Request, userID int64) (string, error) {
	sid := NewSessionID()
	ip := clientIP(r)
	if err := models.CreateSession(m.DB, sid, userID, m.TTL, ip, r.UserAgent()); err != nil {
		return "", err
	}
	m.setCookie(w, sessionCookie, sid, true)
	// CSRF token is readable by JS (not HttpOnly) for the double-submit pattern.
	m.setCookie(w, csrfCookie, NewSessionID(), false)
	return sid, nil
}

// Logout destroys the current session and clears cookies.
func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = models.DeleteSession(m.DB, c.Value)
	}
	m.clearCookie(w, sessionCookie)
	m.clearCookie(w, csrfCookie)
}

// RequireAuth wraps a handler, enforcing a valid session. On failure it
// redirects to /login — or, for htmx requests, emits HX-Redirect so the client
// does a full-page navigation instead of swapping the login page into a panel.
func (m *Manager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := m.currentUser(r)
		if !ok {
			m.denyAuth(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin wraps a handler, enforcing that the logged-in user is an admin.
// Must be used inside RequireAuth.
func (m *Manager) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || !user.IsAdmin() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// VerifyCSRF checks the double-submit token on state-changing requests. Returns
// true if the request may proceed.
func (m *Manager) VerifyCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	got := r.Header.Get(csrfHeader)
	if got == "" {
		got = r.FormValue(csrfField)
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(got)) == 1
}

// CSRFToken returns the current request's CSRF token (for embedding in forms).
func CSRFToken(r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil {
		return c.Value
	}
	return ""
}

// currentUser resolves the session cookie to an active user.
func (m *Manager) currentUser(r *http.Request) (models.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return models.User{}, false
	}
	sess, err := models.GetValidSession(m.DB, c.Value)
	if err != nil {
		return models.User{}, false
	}
	user, err := models.GetUser(m.DB, sess.UserID)
	if err != nil || !user.IsActive {
		return models.User{}, false
	}
	return user, true
}

func (m *Manager) denyAuth(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (m *Manager) setCookie(w http.ResponseWriter, name, value string, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: httpOnly,
		Secure:   m.SecureCook,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(m.TTL),
	})
}

func (m *Manager) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: name == sessionCookie,
		Secure:   m.SecureCook,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// StartSessionGC launches a goroutine that purges expired sessions hourly.
func (m *Manager) StartSessionGC() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := models.DeleteExpiredSessions(m.DB); err != nil {
				log.Printf("session gc: %v", err)
			}
		}
	}()
}

// UserFromContext returns the authenticated user stored by RequireAuth.
func UserFromContext(ctx context.Context) (models.User, bool) {
	u, ok := ctx.Value(userCtxKey).(models.User)
	return u, ok
}

// clientIP extracts a best-effort client IP for audit logging.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}

// ClientIP is the exported form for handlers that audit-log.
func ClientIP(r *http.Request) string { return clientIP(r) }
