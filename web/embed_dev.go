//go:build !prod

package web

import (
	"io/fs"
	"log/slog"
	"os"
)

var Assets fs.FS = os.DirFS("web")

func init() {
	slog.Info("serving web assets from disk", "build_tag", "!prod")
}
