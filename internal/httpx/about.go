package httpx

import (
	"html/template"
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
	renderTemplate(w, h.AboutTmpl, struct{}{})
}
