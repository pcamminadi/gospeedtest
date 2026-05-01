// Package web bundles the static assets for the gospeedtest browser UI.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var assets embed.FS

// FileServer returns an http.Handler that serves the embedded UI from the
// root path. index.html is served for "/".
func FileServer() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
