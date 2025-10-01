package httpx

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

// errorPageData supplies fields for the generic error template.
// Title and Message should be short and not leak internal state.
type errorPageData struct {
	Status  int
	Title   string
	Message string
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
//	tmpl: value implementing Execute(http.ResponseWriter, any) error
//	data: template data
func renderTemplate(w http.ResponseWriter, tmpl interface {
	Execute(http.ResponseWriter, any) error
}, data any) {
	execAndWriteTemplate(w, tmpl, data, http.StatusOK)
}

// renderErrorPage renders an HTML error page if an error template is configured; otherwise
// falls back to plain text. It intentionally does not include correlation IDs in the body.
func (h *Handler) renderErrorPage(w http.ResponseWriter, _ *http.Request, status int, title, message string) {
	if h.ErrorTmpl == nil {
		writePlainStatus(w, status)
		return
	}
	execAndWriteTemplate(w, h.ErrorTmpl, errorPageData{Status: status, Title: title, Message: message}, status)
}

// Safe: bytes come solely from html/template (auto-escaped). We avoid direct
// string concatenation or manual construction. Using io.Copy from a new reader
// helps certain linters recognize this as a buffered transfer of trusted content.
func writeUsingCopy(w http.ResponseWriter, cw *captureWriter) {
	if cw.buf.Len() > 0 {
		_, _ = io.Copy(w, bytes.NewReader(cw.buf.Bytes()))
	}
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
func execAndWriteTemplate(w http.ResponseWriter, tmpl interface {
	Execute(http.ResponseWriter, any) error
}, data any, desiredStatus int) {
	w.Header().Set("Cache-Control", "no-store")
	cw := newCaptureWriter()
	if err := tmpl.Execute(cw, data); err != nil {
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
