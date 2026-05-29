package web

import (
	"net/http"
	"strings"

	"mederos-crm/internal/auth"
	"mederos-crm/internal/models"
)

// handleAdminUsers lists staff accounts.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := models.ListUsers(s.DB)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not load users.")
		return
	}
	s.render(w, r, "admin_users", pageData{
		Title: "Staff accounts",
		Data:  map[string]any{"Users": users, "Flash": r.URL.Query().Get("msg")},
	})
}

// handleAdminCreateUser creates a new staff or admin account. The new user is
// forced to change their password on first login.
func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	username := strings.TrimSpace(r.FormValue("username"))
	display := strings.TrimSpace(r.FormValue("display_name"))
	role := r.FormValue("role")
	pw := r.FormValue("password")

	if role != "admin" {
		role = "staff"
	}
	if len(username) < 3 || len(pw) < 8 {
		s.adminUsersWithFlash(w, r, "Username must be 3+ characters and password 8+ characters.")
		return
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not secure the password.")
		return
	}
	uid, err := models.CreateUser(s.DB, username, hash, role, display, true)
	if err != nil {
		s.adminUsersWithFlash(w, r, "Could not create user (is the username already taken?).")
		return
	}
	_ = models.LogAudit(s.DB, actor.ID, "user.create", "user", itoa(uid), username, auth.ClientIP(r))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// handleAdminResetPassword sets a new password for a user and forces a change.
func (s *Server) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "User not found.")
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	pw := r.FormValue("password")
	if len(pw) < 8 {
		s.adminUsersWithFlash(w, r, "New password must be at least 8 characters.")
		return
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not secure the password.")
		return
	}
	if err := models.SetPassword(s.DB, id, hash); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not reset the password.")
		return
	}
	// Force re-login by clearing the user's sessions.
	_ = models.DeleteUserSessions(s.DB, id)
	_ = models.LogAudit(s.DB, actor.ID, "user.reset_password", "user", itoa(id), "", auth.ClientIP(r))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// handleAdminDeactivate disables a user and ends their sessions.
func (s *Server) handleAdminDeactivate(w http.ResponseWriter, r *http.Request) {
	s.setUserActive(w, r, false)
}

// handleAdminActivate re-enables a user.
func (s *Server) handleAdminActivate(w http.ResponseWriter, r *http.Request) {
	s.setUserActive(w, r, true)
}

func (s *Server) setUserActive(w http.ResponseWriter, r *http.Request, active bool) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "User not found.")
		return
	}
	actor, _ := auth.UserFromContext(r.Context())
	if !active && id == actor.ID {
		s.adminUsersWithFlash(w, r, "You cannot deactivate your own account.")
		return
	}
	if err := models.SetActive(s.DB, id, active); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not update the user.")
		return
	}
	if !active {
		_ = models.DeleteUserSessions(s.DB, id)
	}
	action := "user.activate"
	if !active {
		action = "user.deactivate"
	}
	_ = models.LogAudit(s.DB, actor.ID, action, "user", itoa(id), "", auth.ClientIP(r))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// handleAdminAudit shows the recent audit log.
func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	events, err := models.ListAuditEvents(s.DB, 200)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not load the audit log.")
		return
	}
	s.render(w, r, "admin_audit", pageData{
		Title: "Activity log",
		Data:  map[string]any{"Events": events},
	})
}

func (s *Server) adminUsersWithFlash(w http.ResponseWriter, r *http.Request, msg string) {
	users, _ := models.ListUsers(s.DB)
	s.renderStatus(w, r, "admin_users", pageData{
		Title: "Staff accounts",
		Flash: msg,
		Data:  map[string]any{"Users": users},
	}, http.StatusBadRequest)
}
