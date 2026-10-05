package main

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
	_ "modernc.org/sqlite"
)

// stubIndex implements store.Index minimally for buildService tests.
type stubIndex struct{}

// Insert accepts a new row without persisting it.
//
// Parameters:
//   - context.Context: the request context.
//   - store.NewRow: the row that would be inserted.
//
// Returns:
//   - error: always nil.
func (stubIndex) Insert(context.Context, store.NewRow) error { return nil }

// Claim reports that the requested secret does not exist.
//
// Parameters:
//   - context.Context: the request context.
//   - string: the secret ID.
//   - string: the claim token.
//   - bool: whether attachment metadata should be claimed.
//   - time.Time: the current time.
//   - time.Time: the claim deadline.
//   - store.ExternalOpener: the external payload opener.
//
// Returns:
//   - *store.IndexResult: always nil.
//   - error: os.ErrNotExist.
func (stubIndex) Claim(context.Context, string, string, bool, time.Time, time.Time, store.ExternalOpener) (*store.IndexResult, error) {
	return nil, os.ErrNotExist
}

// Ack reports that the requested claim does not exist.
//
// Parameters:
//   - context.Context: the request context.
//   - string: the secret ID.
//   - string: the claim token.
//
// Returns:
//   - bool: always false.
//   - error: os.ErrNotExist.
func (stubIndex) Ack(context.Context, string, string) (bool, error) { return false, os.ErrNotExist }

// DeleteExpired reports no expired records.
//
// Parameters:
//   - context.Context: the request context.
//   - time.Time: the cutoff time.
//
// Returns:
//   - []store.ExpiredRecord: always nil.
//   - error: always nil.
func (stubIndex) DeleteExpired(context.Context, time.Time) ([]store.ExpiredRecord, error) {
	return nil, nil
}

// ListExternalIDs reports no external IDs.
//
// Parameters:
//   - context.Context: the request context.
//
// Returns:
//   - []string: always nil.
//   - error: always nil.
func (stubIndex) ListExternalIDs(context.Context) ([]string, error) { return nil, nil }

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
func (stubIndex) Status(context.Context, string, string, time.Time) (app.SecretStatus, error) {
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
func (stubIndex) Revoke(context.Context, string, string, time.Time) (bool, error) {
	return false, app.ErrNotFound
}

// stubBlobStorage implements store.BlobStorage.
type stubBlobStorage struct{}

// Write accepts a blob without storing it.
//
// Parameters:
//   - string: the blob ID.
//   - io.Reader: the blob payload.
//   - int64: the payload size.
//
// Returns:
//   - error: always nil.
func (stubBlobStorage) Write(string, io.Reader, int64) error { return nil }

// Open reports that the requested blob does not exist.
//
// Parameters:
//   - string: the blob ID.
//
// Returns:
//   - io.ReadCloser: always nil.
//   - error: os.ErrNotExist.
func (stubBlobStorage) Open(string) (io.ReadCloser, error) { return nil, os.ErrNotExist }

// Delete accepts a delete request without storing state.
//
// Parameters:
//   - string: the blob ID.
//
// Returns:
//   - error: always nil.
func (stubBlobStorage) Delete(string) error { return nil }

// List reports no stored blobs.
//
// Returns:
//   - []string: always nil.
//   - error: always nil.
func (stubBlobStorage) List() ([]string, error) { return nil, nil }

type recordingIndex struct {
	inline   []byte
	external bool
}

// Insert records whether the row used inline or external storage.
//
// Parameters:
//   - context.Context: the request context.
//   - row: the row being inserted.
//
// Returns:
//   - error: always nil.
func (r *recordingIndex) Insert(_ context.Context, row store.NewRow) error {
	r.inline = row.Inline
	r.external = row.External
	return nil
}

// Claim reports that the requested secret does not exist.
//
// Parameters:
//   - context.Context: the request context.
//   - string: the secret ID.
//   - string: the claim token.
//   - bool: whether attachment metadata should be claimed.
//   - time.Time: the current time.
//   - time.Time: the claim deadline.
//   - store.ExternalOpener: the external payload opener.
//
// Returns:
//   - *store.IndexResult: always nil.
//   - error: os.ErrNotExist.
func (r *recordingIndex) Claim(context.Context, string, string, bool, time.Time, time.Time, store.ExternalOpener) (*store.IndexResult, error) {
	return nil, os.ErrNotExist
}

// Ack reports that the requested claim does not exist.
//
// Parameters:
//   - context.Context: the request context.
//   - string: the secret ID.
//   - string: the claim token.
//
// Returns:
//   - bool: always false.
//   - error: os.ErrNotExist.
func (r *recordingIndex) Ack(context.Context, string, string) (bool, error) {
	return false, os.ErrNotExist
}

// DeleteExpired reports no expired records.
//
// Parameters:
//   - context.Context: the request context.
//   - time.Time: the cutoff time.
//
// Returns:
//   - []store.ExpiredRecord: always nil.
//   - error: always nil.
func (r *recordingIndex) DeleteExpired(context.Context, time.Time) ([]store.ExpiredRecord, error) {
	return nil, nil
}
