package admin

import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed static/*
var uiFS embed.FS

// Handler serves the embedded admin console static files.
func Handler() http.Handler {
    sub, _ := fs.Sub(uiFS, "static")
    return http.FileServer(http.FS(sub))
}

