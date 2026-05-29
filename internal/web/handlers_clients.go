package web

import (
	"net/http"
	"strconv"
	"strings"

	"mederos-crm/internal/auth"
	"mederos-crm/internal/models"
)

// handleClientsList renders the searchable client list. When called by htmx
// (HX-Request header), it returns just the table fragment for live search.
func (s *Server) handleClientsList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	clients, err := models.ListClients(s.DB, q)
	if err != nil {
		s.logf("list clients: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Could not load clients.")
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		s.renderPartial(w, "clients_table", map[string]any{"Clients": clients, "Query": q})
		return
	}
	s.render(w, r, "clients_list", pageData{
		Title: "Clients",
		Data:  map[string]any{"Clients": clients, "Query": q},
	})
}

// handleClientDetail shows one client's full record.
func (s *Server) handleClientDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	c, err := models.GetClient(s.DB, id)
	if err != nil {
		if isNotFound(err) {
			s.renderError(w, r, http.StatusNotFound, "Client not found.")
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, "Could not load the client.")
		return
	}
	s.render(w, r, "client_detail", pageData{
		Title: c.Name,
		Data: map[string]any{
			"Client":       c,
			"FilesEnabled": s.browser != nil && c.HasFolder(),
		},
	})
}

// handleClientNewForm shows an empty create form.
func (s *Server) handleClientNewForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "client_form", pageData{
		Title: "Add client",
		Data:  map[string]any{"Client": models.Client{}, "IsNew": true},
	})
}

// handleClientCreate inserts a new client.
func (s *Server) handleClientCreate(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	c := clientFromForm(r)
	if c.Name == "" {
		s.renderStatus(w, r, "client_form", pageData{
			Title: "Add client",
			Flash: "A client name is required.",
			Data:  map[string]any{"Client": c, "IsNew": true},
		}, http.StatusBadRequest)
		return
	}
	id, err := models.CreateClient(s.DB, c, user.ID)
	if err != nil {
		s.logf("create client: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Could not save the client.")
		return
	}
	_ = models.LogAudit(s.DB, user.ID, "client.create", "client", itoa(id), c.Name, auth.ClientIP(r))
	http.Redirect(w, r, "/clients/"+itoa(id), http.StatusSeeOther)
}

// handleClientEditForm shows the edit form populated with existing values.
func (s *Server) handleClientEditForm(w http.ResponseWriter, r *http.Request) {
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
	s.render(w, r, "client_form", pageData{
		Title: "Edit " + c.Name,
		Data:  map[string]any{"Client": c, "IsNew": false},
	})
}

// handleClientUpdate saves edits to an existing client.
func (s *Server) handleClientUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	c := clientFromForm(r)
	c.ID = id
	if c.Name == "" {
		s.renderStatus(w, r, "client_form", pageData{
			Title: "Edit client",
			Flash: "A client name is required.",
			Data:  map[string]any{"Client": c, "IsNew": false},
		}, http.StatusBadRequest)
		return
	}
	if err := models.UpdateClient(s.DB, c); err != nil {
		s.logf("update client: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Could not save the client.")
		return
	}
	_ = models.LogAudit(s.DB, user.ID, "client.update", "client", itoa(id), c.Name, auth.ClientIP(r))
	http.Redirect(w, r, "/clients/"+itoa(id), http.StatusSeeOther)
}

// handleClientDelete soft-deletes a client.
func (s *Server) handleClientDelete(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.VerifyCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Invalid request token. Please try again.")
		return
	}
	id, ok := parseID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Client not found.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	if err := models.SoftDeleteClient(s.DB, id); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not delete the client.")
		return
	}
	_ = models.LogAudit(s.DB, user.ID, "client.delete", "client", itoa(id), "", auth.ClientIP(r))
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

// clientFromForm builds a Client from posted form values (trimmed).
func clientFromForm(r *http.Request) models.Client {
	return models.Client{
		Name:       strings.TrimSpace(r.FormValue("name")),
		Address:    strings.TrimSpace(r.FormValue("address")),
		Phone:      strings.TrimSpace(r.FormValue("phone")),
		Email:      strings.TrimSpace(r.FormValue("email")),
		Birthday:   strings.TrimSpace(r.FormValue("birthday")),
		CaseType:   strings.TrimSpace(r.FormValue("case_type")),
		Adversary:  strings.TrimSpace(r.FormValue("adversary")),
		Notes:      strings.TrimSpace(r.FormValue("notes")),
		FolderPath: strings.TrimSpace(r.FormValue("folder_path")),
	}
}

// parseID reads the {id} path value as an int64.
func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
