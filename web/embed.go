package web

import "embed"

// FS contains the embedded web templates (index) and static assets.
//
//go:embed *.tmpl.html css/* js/*
var FS embed.FS
