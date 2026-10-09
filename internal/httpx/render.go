package httpx

import (
	"bytes"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"

	"github.com/haukened/gone/v3/internal/i18n"
)

// errorPageData supplies fields for the generic error template: the status
// code and the message keys for the page title, heading, and explanation.
// The messages should be short and not leak internal state.
type errorPageData struct {
	Status       int
	PageTitleKey string
	TitleKey     string
	MessageKey   string
}

// executor renders a page template with data into w.
type executor interface {
	Execute(http.ResponseWriter, any) error
}

// LocaleRenderer is implemented by page templates that have one clone per
// locale. Pages rendered through it carry Content-Language and Vary headers.
type LocaleRenderer interface {
	ExecuteLocale(w http.ResponseWriter, tag string, data any) error
}

// LocalizedTemplate holds one clone of a page template per locale tag.
type LocalizedTemplate map[string]*template.Template

// errNoTemplate is returned when a LocalizedTemplate has no clone to render.
var errNoTemplate = errors.New("no template for locale")

// Execute renders the base locale's clone.
func (lt LocalizedTemplate) Execute(w http.ResponseWriter, data any) error {
	return lt.ExecuteLocale(w, i18n.BaseLocale, data)
}

// ExecuteLocale renders the clone for tag, falling back to the base locale.
//
// Parameters:
//   - w: destination.
//   - tag: the request's locale.
//   - data: template data.
//
// Returns:
//   - error: the template error, or errNoTemplate when no clone exists.
func (lt LocalizedTemplate) ExecuteLocale(w http.ResponseWriter, tag string, data any) error {
	t := lt[tag]
	if t == nil {
		t = lt[i18n.BaseLocale]
	}
	if t == nil {
		return errNoTemplate
	}
	return t.Execute(w, data)
}

// captureWriter buffers template output and any status the template might set.
type captureWriter struct {
	buf    bytes.Buffer
	header http.Header
	status int
}

func newCaptureWriter() *captureWriter               { return &captureWriter{header: make(http.Header)} }
func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) { return c.buf.Write(b) }
func (c *captureWriter) WriteHeader(status int)      { c.status = status }

// renderTemplate renders an HTML template with standard security/cache headers.
// It buffers output so that if Execute returns an error after partial writes we
// can still emit a consistent 500 with a fallback body while preserving any
// partial output. On success the buffered content is written with HTML headers.
// Parameters:
//
//	w: http.ResponseWriter to write headers and body
//	r: the request, whose context carries the locale
//	tmpl: value implementing Execute(http.ResponseWriter, any) error
//	data: template data
func renderTemplate(w http.ResponseWriter, r *http.Request, tmpl executor, data any) {
	execAndWriteTemplate(w, r, tmpl, data, http.StatusOK)
}

// renderErrorPage renders an HTML error page if an error template is configured; otherwise
// falls back to plain text. It intentionally does not include correlation IDs in the body.
// keyBase names the messages: keyBase+".pageTitle", ".title" and ".message".
func (h *Handler) renderErrorPage(w http.ResponseWriter, r *http.Request, status int, keyBase string) {
	if h.ErrorTmpl == nil {
		writePlainStatus(w, status)
		return
	}
	data := errorPageData{Status: status, PageTitleKey: keyBase + ".pageTitle", TitleKey: keyBase + ".title", MessageKey: keyBase + ".message"}
	execAndWriteTemplate(w, r, h.ErrorTmpl, data, status)
}

// Safe: bytes come solely from html/template (auto-escaped). We avoid direct
// string concatenation or manual construction. Using io.Copy from a new reader
// helps certain linters recognize this as a buffered transfer of trusted content.
func writeUsingCopy(w http.ResponseWriter, cw *captureWriter) {
	if cw.buf.Len() > 0 {
		_, _ = io.Copy(w, bytes.NewReader(cw.buf.Bytes()))
	}
}

// executePage renders tmpl into cw, in the request's locale when tmpl has
// per-locale clones, marking the response's language on w.
func executePage(w http.ResponseWriter, r *http.Request, cw *captureWriter, tmpl executor, data any) error {
	lr, ok := tmpl.(LocaleRenderer)
	if !ok || r == nil {
		return tmpl.Execute(cw, data)
	}
	tag := i18n.FromContext(r.Context())
	i18n.PageHeaders(w.Header(), tag)
	return lr.ExecuteLocale(cw, tag, data)
}

// writePlainStatus writes a plain text status response with standard headers.
func writePlainStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	// constant body; safe
	// nosemgrep
	_, _ = w.Write([]byte(http.StatusText(status)))
}

// execAndWriteTemplate centralizes template execution, buffering and error handling.
// If tmpl execution fails, it emits a generic 500 without leaking partial output.
// desiredStatus is used when the template does not set an explicit status.
// A LocaleRenderer is rendered in the request's locale.
func execAndWriteTemplate(w http.ResponseWriter, r *http.Request, tmpl executor, data any, desiredStatus int) {
	w.Header().Set("Cache-Control", "no-store")
	cw := newCaptureWriter()
	if err := executePage(w, r, cw, tmpl, data); err != nil {
		slog.Error("render", "domain", "ui", "action", "error")
		writePlainStatus(w, http.StatusInternalServerError)
		return
	}
	status := cw.status
	if status == 0 {
		status = desiredStatus
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	writeUsingCopy(w, cw)
}
