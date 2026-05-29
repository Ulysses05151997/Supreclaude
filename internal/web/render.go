package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"mederos-crm/internal/auth"
	"mederos-crm/internal/models"
)

// funcMap holds template helpers shared by all templates.
var funcMap = template.FuncMap{
	"humanSize": humanSize,
	"shortTime": shortTime,
	"nl2br":     nl2br,
	"trimRoot":  trimRoot,
}

// pages maps a logical page name to a parsed template (layout + page body).
// partials maps a fragment name to a standalone template for htmx responses.
type renderer struct {
	pages    map[string]*template.Template
	partials map[string]*template.Template
}

// newRenderer parses every page template (each combined with the layout) and
// every partial from the embedded template FS.
func newRenderer(tplFS fs.FS) (*renderer, error) {
	r := &renderer{
		pages:    map[string]*template.Template{},
		partials: map[string]*template.Template{},
	}

	pageNames := []string{
		"login", "setup", "change_password",
		"clients_list", "client_detail", "client_form",
		"admin_users", "admin_audit", "error",
	}
	for _, name := range pageNames {
		// Each page is parsed together with the layout and all partials, so a
		// page can {{template "clients_table.html" .}} the same fragment that
		// htmx swaps in standalone.
		t, err := template.New("layout.html").Funcs(funcMap).ParseFS(
			tplFS, "layout.html", name+".html", "partials/*.html")
		if err != nil {
			return nil, fmt.Errorf("parsing page %q: %w", name, err)
		}
		r.pages[name] = t
	}

	partialNames := []string{"clients_table", "files_panel"}
	for _, name := range partialNames {
		t, err := template.New(name+".html").Funcs(funcMap).ParseFS(
			tplFS, "partials/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parsing partial %q: %w", name, err)
		}
		r.partials[name] = t
	}
	return r, nil
}

// pageData is the standard template context. Handlers embed it (or set Data to
// a page-specific struct).
type pageData struct {
	Title     string
	User      models.User
	LoggedIn  bool
	CSRFToken string
	Flash     string
	Data      any
}

// render writes a full page (layout + body) with status 200.
func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data pageData) {
	s.renderStatus(w, r, page, data, http.StatusOK)
}

func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, page string, data pageData, status int) {
	t, ok := s.rnd.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	if r != nil {
		if u, ok := auth.UserFromContext(r.Context()); ok {
			data.User = u
			data.LoggedIn = true
		}
		if data.CSRFToken == "" {
			data.CSRFToken = auth.CSRFToken(r)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
		// Response is likely already partially written; log via standard logger.
		fmt.Printf("render error (%s): %v\n", page, err)
	}
}

// renderPartial writes a fragment template (for htmx swaps).
func (s *Server) renderPartial(w http.ResponseWriter, name string, data any) {
	t, ok := s.rnd.partials[name]
	if !ok {
		http.Error(w, "partial not found: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		fmt.Printf("render partial error (%s): %v\n", name, err)
	}
}

// renderError shows a friendly error page.
func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.renderStatus(w, r, "error", pageData{
		Title: "Error",
		Data:  map[string]any{"Status": status, "Message": msg},
	}, status)
}

// ---- template helpers ----

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// shortTime trims the seconds/timezone from the DB's "2006-01-02 15:04:05" text.
func shortTime(s string) string {
	if len(s) >= 16 {
		return s[:16]
	}
	return s
}

// nl2br escapes text then converts newlines to <br> for safe display of notes.
func nl2br(s string) template.HTML {
	escaped := template.HTMLEscapeString(s)
	return template.HTML(strings.ReplaceAll(escaped, "\n", "<br>"))
}

// trimRoot returns the portion of a root-relative entry path that lies below
// the client's folder root — i.e. the ?sub= value used to browse into it.
func trimRoot(relPath, root string) string {
	rel := strings.TrimPrefix(relPath, root)
	return strings.TrimPrefix(rel, "/")
}
