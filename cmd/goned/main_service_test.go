package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/store"
	wembed "github.com/haukened/gone/v3/web"
)

// ListExternalIDs reports no external IDs.
//
// Parameters:
//   - context.Context: the request context.
//
// Returns:
//   - []string: always nil.
//   - error: always nil.
func (r *recordingIndex) ListExternalIDs(context.Context) ([]string, error) { return nil, nil }

// Status reports that the requested secret does not exist.
//
// Parameters:
//   - context.Context: the request context.
//   - string: the secret ID.
//   - string: the management token.
//   - time.Time: the current time.
//
// Returns:
//   - app.SecretStatus: the zero status.
//   - error: app.ErrNotFound.
func (r *recordingIndex) Status(context.Context, string, string, time.Time) (app.SecretStatus, error) {
	return app.SecretStatus{}, app.ErrNotFound
}

// Revoke reports that the requested secret does not exist.
//
// Parameters:
//   - context.Context: the request context.
//   - string: the secret ID.
//   - string: the management token.
//   - time.Time: the current time.
//
// Returns:
//   - bool: always false.
//   - error: app.ErrNotFound.
func (r *recordingIndex) Revoke(context.Context, string, string, time.Time) (bool, error) {
	return false, app.ErrNotFound
}

type recordingBlobStorage struct {
	wrote bool
	data  []byte
}

// Write records the blob payload it receives.
//
// Parameters:
//   - string: the blob ID.
//   - src: the blob payload.
//   - int64: the payload size.
//
// Returns:
//   - error: any read error from src.
func (r *recordingBlobStorage) Write(_ string, src io.Reader, _ int64) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	r.wrote = true
	r.data = data
	return nil
}

// Open reports that the requested blob does not exist.
//
// Parameters:
//   - string: the blob ID.
//
// Returns:
//   - io.ReadCloser: always nil.
//   - error: os.ErrNotExist.
func (r *recordingBlobStorage) Open(string) (io.ReadCloser, error) { return nil, os.ErrNotExist }

// Delete accepts a delete request without storing state.
//
// Parameters:
//   - string: the blob ID.
//
// Returns:
//   - error: always nil.
func (r *recordingBlobStorage) Delete(string) error { return nil }

// List reports no stored blobs.
//
// Returns:
//   - []string: always nil.
//   - error: always nil.
func (r *recordingBlobStorage) List() ([]string, error) { return nil, nil }

// TestLoadTemplates ensures embedded templates can be loaded.
//
// Parameters:
//   - t: the test handle.
func TestLoadTemplates(t *testing.T) {
	tmpls, err := loadTemplatesFrom(wembed.Assets, "v0.0.0-test")
	if err != nil {
		t.Fatalf("loadTemplatesFrom error: %v", err)
	}
	if tmpls.index == nil || tmpls.about == nil || tmpls.secret == nil || tmpls.manage == nil || tmpls.errorPage == nil {
		t.Fatalf("expected all templates non-nil")
	}
}

// TestBuildService validates service field propagation.
//
// Parameters:
//   - t: the test handle.
func TestBuildService(t *testing.T) {
	cfg := &config.Config{InlineMaxBytes: 64, MaxBytes: 1234, MinTTL: time.Minute, MaxTTL: 2 * time.Minute, ClaimLease: 75 * time.Second}
	s := buildService(stubIndex{}, stubBlobStorage{}, cfg, realClock{})
	if s.MaxBytes != 1234 {
		t.Fatalf("MaxBytes mismatch got %d", s.MaxBytes)
	}
	if s.MinTTL != time.Minute || s.MaxTTL != 2*time.Minute {
		t.Fatalf("TTL mismatch")
	}
	if s.ClaimLease != 75*time.Second {
		t.Fatalf("ClaimLease mismatch got %v", s.ClaimLease)
	}
}

// TestBuildServiceUsesConfiguredInlineThreshold validates blob threshold wiring.
//
// Parameters:
//   - t: the test handle.
func TestBuildServiceUsesConfiguredInlineThreshold(t *testing.T) {
	idx := &recordingIndex{}
	blobs := &recordingBlobStorage{}
	cfg := &config.Config{InlineMaxBytes: 4, MaxBytes: 32, MinTTL: time.Minute, MaxTTL: 2 * time.Minute}
	s := buildService(idx, blobs, cfg, realClock{})

	_, err := s.CreateSecret(context.Background(), strings.NewReader("external"), int64(len("external")), 1, "AAAAAAAAAAAAAAAA", time.Minute)
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if !blobs.wrote {
		t.Fatalf("expected payload to be written to blob storage")
	}
	if !idx.external {
		t.Fatalf("expected index insert to mark secret external")
	}
	if len(idx.inline) != 0 {
		t.Fatalf("expected no inline payload, got %d bytes", len(idx.inline))
	}
}

// TestNewServer ensures timeouts and addr applied.
//
// Parameters:
//   - t: the test handle.
func TestNewServer(t *testing.T) {
	cfg := &config.Config{Addr: ":9999"}
	srv := newServer(cfg, http.NewServeMux())
	if srv.Addr != ":9999" {
		t.Fatalf("addr mismatch got %s", srv.Addr)
	}
	if srv.ReadTimeout == 0 || srv.WriteTimeout == 0 || srv.ReadHeaderTimeout == 0 {
		t.Fatalf("expected non-zero timeouts")
	}
	if srv.ReadHeaderTimeout >= srv.ReadTimeout {
		t.Fatalf("expected header timeout (%v) tighter than body timeout (%v)", srv.ReadHeaderTimeout, srv.ReadTimeout)
	}
}

var _ store.Index = (*recordingIndex)(nil)
var _ store.BlobStorage = (*recordingBlobStorage)(nil)
