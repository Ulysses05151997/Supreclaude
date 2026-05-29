package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"mederos-crm/internal/auth"
	"mederos-crm/internal/models"
)

// handleIndex redirects the root to the client list.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

// handleLoginForm shows the login page (or redirects to setup on first run).
func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if s.needsSetup() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	s.render(w, r, "login", pageData{Title: "Sign in"})
}

// handleLoginSubmit authenticates a user and starts a session.
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	user, err := models.GetUserByUsername(s.DB, username)
	if err != nil {
		// Equalize timing so we don't reveal whether the account exists.
		auth.CheckDummy(password)
		s.renderStatus(w, r, "login", pageData{
			Title: "Sign in",
			Flash: "Incorrect username or password.",
		}, http.StatusUnauthorized)
		return
	}
	if !user.IsActive || !auth.CheckPassword(user.PasswordHash, password) {
		s.renderStatus(w, r, "login", pageData{
			Title: "Sign in",
			Flash: "Incorrect username or password.",
		}, http.StatusUnauthorized)
		return
	}

	if _, err := s.Auth.Login(w, r, user.ID); err != nil {
		s.logf("login: create session: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Could not start your session.")
		return
	}
	_ = models.LogAudit(s.DB, user.ID, "login", "user", itoa(user.ID), "", auth.ClientIP(r))

	if user.MustChangePW {
		http.Redirect(w, r, "/change-password", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

// handleLogout destroys the session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	s.Auth.Logout(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleSetupForm shows first-run admin creation, only when no users exist.
func (s *Server) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	if !s.needsSetup() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, r, "setup", pageData{Title: "Welcome — Create the first account"})
}

// handleSetupSubmit creates the first admin account inside a guard transaction.
func (s *Server) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.needsSetup() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	display := strings.TrimSpace(r.FormValue("display_name"))
	pw := r.FormValue("password")
	pw2 := r.FormValue("password_confirm")

	if msg := validateNewCredentials(username, pw, pw2); msg != "" {
		s.renderStatus(w, r, "setup", pageData{Title: "Welcome", Flash: msg}, http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(pw)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not secure the password.")
		return
	}

	// Re-check the count inside a transaction to avoid a setup race.
	tx, err := s.DB.Begin()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Database error.")
		return
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n > 0 {
		tx.Rollback()
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	res, err := tx.Exec(`INSERT INTO users (username, password_hash, role, display_name)
		VALUES (?,?,'admin',?)`, username, hash, display)
	if err != nil {
		tx.Rollback()
		s.renderStatus(w, r, "setup", pageData{
			Title: "Welcome",
			Flash: "Could not create the account (is the username already taken?).",
		}, http.StatusBadRequest)
		return
	}
	if err := tx.Commit(); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Database error.")
		return
	}

	uid, _ := res.LastInsertId()
	_ = models.LogAudit(s.DB, uid, "setup.admin_created", "user", itoa(uid), "", auth.ClientIP(r))
	if _, err := s.Auth.Login(w, r, uid); err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

// handleChangePasswordForm shows the self-service password change page.
func (s *Server) handleChangePasswordForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "change_password", pageData{Title: "Change password"})
}

// handleChangePasswordSubmit lets the logged-in user set a new password.
func (s *Server) handleChangePasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	current := r.FormValue("current_password")
	pw := r.FormValue("password")
	pw2 := r.FormValue("password_confirm")

	if !auth.CheckPassword(user.PasswordHash, current) {
		s.renderStatus(w, r, "change_password", pageData{
			Title: "Change password", Flash: "Your current password is incorrect.",
		}, http.StatusBadRequest)
		return
	}
	if msg := validatePassword(pw, pw2); msg != "" {
		s.renderStatus(w, r, "change_password", pageData{
			Title: "Change password", Flash: msg,
		}, http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not secure the password.")
		return
	}
	if err := models.SetPassword(s.DB, user.ID, hash); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not save the password.")
		return
	}
	_ = models.LogAudit(s.DB, user.ID, "password.change", "user", itoa(user.ID), "", auth.ClientIP(r))
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

// needsSetup reports whether zero users exist (first-run state).
func (s *Server) needsSetup() bool {
	n, err := models.CountUsers(s.DB)
	if err != nil {
		s.logf("count users: %v", err)
		return false
	}
	return n == 0
}

// validateNewCredentials checks username + password rules for account creation.
func validateNewCredentials(username, pw, pw2 string) string {
	if len(username) < 3 {
		return "Username must be at least 3 characters."
	}
	return validatePassword(pw, pw2)
}

func validatePassword(pw, pw2 string) string {
	if len(pw) < 8 {
		return "Password must be at least 8 characters."
	}
	if pw != pw2 {
		return "The two passwords do not match."
	}
	return ""
}

// errNoRows is a small alias to keep handler intent readable.
var errNoRows = sql.ErrNoRows

func isNotFound(err error) bool { return errors.Is(err, errNoRows) }
