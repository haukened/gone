package httpx

import (
	"html/template"
	"io/fs"
	"net/http"
)

// AboutRenderer abstracts template execution for the about page.
// It mirrors IndexRenderer for symmetry and testability.
type AboutRenderer interface {
	Execute(w http.ResponseWriter, data any) error
}

// AboutTemplateRenderer implements AboutRenderer using html/template.
type AboutTemplateRenderer struct{ T *template.Template }

// Execute writes the rendered about template to the ResponseWriter.
func (tr AboutTemplateRenderer) Execute(w http.ResponseWriter, data any) error {
	return tr.T.Execute(w, data)
}

// handleAbout renders the informational /about page.
// It returns 503 if the template is unavailable, and 404 if an unexpected path is routed here.
// The about page is static today (no dynamic data) but a placeholder struct is retained
// for potential future metrics or configuration exposure.
func (h *Handler) handleAbout(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/about" { // exact match only
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}
	if h.AboutTmpl == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("about unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.AboutTmpl.Execute(w, struct{}{}); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("template error"))
	}
}

// parseAboutTemplate loads about.tmpl.html from an fs.FS and returns a parsed template.
// This helper is used by tests to avoid duplicating parsing logic.
func parseAboutTemplate(fsys fs.FS) (*template.Template, error) {
	b, err := fs.ReadFile(fsys, "about.tmpl.html")
	if err != nil {
		return nil, err
	}
	return template.New("about").Parse(string(b))
}
