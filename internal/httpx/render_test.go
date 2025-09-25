package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockTemplate implements the minimal interface required by renderTemplate.
// It can be configured to write output, return an error, or both.
type mockTemplate struct {
	writeBody    string
	err          error
	writePartial string
}

// Execute writes optional partial + body then returns the configured error.
func (m *mockTemplate) Execute(w http.ResponseWriter, _ any) error {
	if m.writePartial != "" {
		_, _ = w.Write([]byte(m.writePartial))
	}
	if m.writeBody != "" {
		_, _ = w.Write([]byte(m.writeBody))
	}
	return m.err
}

func TestRenderTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tmpl           *mockTemplate
		wantStatus     int
		wantBodySubstr string
		wantCT         string
	}{
		{
			name:           "success_writes_html_and_headers",
			tmpl:           &mockTemplate{writeBody: "<h1>ok</h1>"},
			wantStatus:     http.StatusOK,
			wantBodySubstr: "<h1>ok</h1>",
			wantCT:         "text/html; charset=utf-8",
		},
		{
			name:           "error_no_prior_write_sets_500_and_plain_text",
			tmpl:           &mockTemplate{err: errors.New("boom")},
			wantStatus:     http.StatusInternalServerError,
			wantBodySubstr: "template error",
			wantCT:         "text/plain; charset=utf-8",
		},
		{
			name:           "error_after_partial_write_overrides_status_and_content_type",
			tmpl:           &mockTemplate{writePartial: "<p>partial</p>", err: errors.New("later failure")},
			wantStatus:     http.StatusInternalServerError,
			wantBodySubstr: "template error", // fallback body (partial discarded for security)
			wantCT:         "text/plain; charset=utf-8",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rr := httptest.NewRecorder()

			renderTemplate(rr, tc.tmpl, nil)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d want %d", rr.Code, tc.wantStatus)
			}

			ct := rr.Header().Get("Content-Type")
			if ct != tc.wantCT {
				t.Fatalf("content-type = %q want %q", ct, tc.wantCT)
			}

			// Cache-Control must always be no-store (even on error).
			if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
				t.Fatalf("cache-control = %q want %q", cc, "no-store")
			}

			body := rr.Body.String()
			if !strings.Contains(body, tc.wantBodySubstr) {
				t.Fatalf("body %q does not contain %q", body, tc.wantBodySubstr)
			}

			// Partial output is intentionally discarded on error for security; no assertion here.
		})
	}
}

func TestNewCaptureWriter(t *testing.T) {
	t.Parallel()
	cw := newCaptureWriter()
	if cw == nil {
		t.Fatal("newCaptureWriter returned nil")
	}
	if cw.status != 0 {
		t.Fatalf("expected initial status 0 got %d", cw.status)
	}
	if cw.buf.Len() != 0 {
		t.Fatalf("expected empty buffer got %d bytes", cw.buf.Len())
	}
	if cw.header == nil {
		t.Fatal("expected non-nil header map")
	}
	if len(cw.header) != 0 {
		t.Fatalf("expected empty header map got len=%d", len(cw.header))
	}
	// Header() should return same map allowing mutation.
	h := cw.Header()
	h.Set("X-Test", "v")
	if got := cw.header.Get("X-Test"); got != "v" {
		t.Fatalf("header mutation not reflected, got %q", got)
	}
}

func TestCaptureWriterWriteAndStatus(t *testing.T) {
	t.Parallel()
	cw := newCaptureWriter()

	tests := [][]byte{
		[]byte("hello"),
		[]byte(" "),
		[]byte("world"),
	}

	total := 0
	for _, part := range tests {
		n, err := cw.Write(part)
		if err != nil {
			t.Fatalf("write error: %v", err)
		}
		if n != len(part) {
			t.Fatalf("write returned %d want %d", n, len(part))
		}
		total += len(part)
	}

	if cw.buf.Len() != total {
		t.Fatalf("buffer length %d want %d", cw.buf.Len(), total)
	}
	if got := cw.buf.String(); got != "hello world" {
		t.Fatalf("buffer content %q want %q", got, "hello world")
	}

	// Status should still be zero until explicitly set.
	if cw.status != 0 {
		t.Fatalf("unexpected initial status %d", cw.status)
	}

	cw.WriteHeader(418)
	if cw.status != 418 {
		t.Fatalf("status = %d want 418", cw.status)
	}

	// Second call overwrites (current implementation); verify behavior.
	cw.WriteHeader(201)
	if cw.status != 201 {
		t.Fatalf("status overwrite = %d want 201", cw.status)
	}
}
