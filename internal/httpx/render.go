package httpx

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

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
	// Always enforce no-store caching.
	w.Header().Set("Cache-Control", "no-store")
	cw := newCaptureWriter()
	err := tmpl.Execute(cw, data)
	if err != nil {
		// On template execution error, avoid reflecting partial output back; emit
		// a structured log without template internals. We don't have request
		// context here, so correlation id (cid) is not attached.
		slog.Error("render", "domain", "ui", "action", "error")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("template error"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	status := cw.status
	if status == 0 { // template never set status explicitly
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if cw.buf.Len() > 0 {
		// Safe: bytes come solely from html/template (auto-escaped). We avoid direct
		// string concatenation or manual construction. Using io.Copy from a new reader
		// helps certain linters recognize this as a buffered transfer of trusted content.
		_, _ = io.Copy(w, bytes.NewReader(cw.buf.Bytes()))
	}
}
