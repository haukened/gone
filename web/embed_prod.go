//go:build prod

package web

import (
	"embed"
	"io/fs"
	"log/slog"
)

//go:embed dist/**
var embedded embed.FS

var Assets fs.FS

func init() {
	slog.Info("serving web assets from embedded filesystem", "build_tag", "prod")
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	Assets = sub
}
