package filesystem

import "os"

// syncBlobDir is the directory sync used by Write; tests replace it to
// exercise a failed sync.
var syncBlobDir = syncDir

// syncDir flushes a directory's entries to stable storage, making a file just
// created in it survive a crash.
//
// Parameters:
//   - dir: directory to sync.
//
// Returns an error if the directory cannot be opened, synced, or closed.
func syncDir(dir string) error {
	d, err := os.Open(dir) // #nosec G304 -- dir is the blob store's fixed root
	if err != nil {
		return err
	}
	return closeBlobFile(d, d.Sync())
}
