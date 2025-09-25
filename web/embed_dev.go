//go:build !prod

package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// Assets is a filesystem rooted at the web/ package directory, independent
// of the process working directory. This makes both `go run ./cmd/gone` and
// `go test ./web` behave consistently.
var Assets fs.FS

func init() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed in web/embed_dev.go")
	}
	dir := filepath.Dir(file) // absolute path to the web package dir
	Assets = os.DirFS(dir)
}
