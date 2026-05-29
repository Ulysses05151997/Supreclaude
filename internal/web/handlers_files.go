package web

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"mederos-crm/internal/auth"
	"mederos-crm/internal/files"
	"mederos-crm/internal/models"
)

// handleClientFiles renders the case-files panel (a directory listing) for a
// client. Returned as an htmx fragment. The ?sub= param browses subfolders.
func (s *Server) handleClientFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	c, err := models.GetClient(s.DB, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	if s.browser == nil || !c.HasFolder() {
		s.renderPartial(w, "files_panel", map[string]any{
			"Client":  c,
			"Error":   "No case-files folder is linked to this client.",
			"Entries": nil,
		})
		return
	}

	sub := strings.TrimSpace(r.URL.Query().Get("sub"))
	entries, dirRel, err := s.browser.List(c.FolderPath, sub)
	if err != nil {
		msg := "Could not read the case-files folder."
		if errors.Is(err, files.ErrUnsafePath) {
			msg = "That location is not allowed."
		}
		s.renderPartial(w, "files_panel", map[string]any{
			"Client": c, "Error": msg, "Entries": nil,
		})
		return
	}

	// Compute a parent "up" link if we're below the client's folder root.
	clientRoot := path.Clean(strings.TrimSpace(c.FolderPath))
	var parentSub string
	hasParent := false
	if dirRel != "" && dirRel != clientRoot {
		parent := path.Dir(dirRel)
		hasParent = true
		// parentSub is the part of the parent below the client folder root.
		parentSub = strings.TrimPrefix(parent, clientRoot)
		parentSub = strings.TrimPrefix(parentSub, "/")
	}

	s.renderPartial(w, "files_panel", map[string]any{
		"Client":    c,
		"Entries":   entries,
		"CurrentRel": dirRel,
		"HasParent": hasParent,
		"ParentSub": parentSub,
		"ClientRoot": clientRoot,
	})
}

// handleClientFileDownload streams a file from the client's folder.
func (s *Server) handleClientFileDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	c, err := models.GetClient(s.DB, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	if s.browser == nil || !c.HasFolder() {
		s.renderError(w, r, http.StatusNotFound, "No files are available for this client.")
		return
	}

	relPath := r.URL.Query().Get("path")
	if err := s.browser.Serve(w, r, c.FolderPath, relPath); err != nil {
		if errors.Is(err, files.ErrUnsafePath) {
			s.renderError(w, r, http.StatusForbidden, "That file is not allowed.")
			return
		}
		s.renderError(w, r, http.StatusNotFound, "File not found.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	_ = models.LogAudit(s.DB, user.ID, "file.download", "client", itoa(id), relPath, auth.ClientIP(r))
}
