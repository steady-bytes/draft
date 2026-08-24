package main

import (
	"embed"
	"io/fs"
)

// rawStaticFS embeds static/htmx.min.js (vendored locally, same copy
// services/tooling/garage uses — see that service's static.go for the
// rationale) into the compiled binary.
//
//go:embed static/htmx.min.js
var rawStaticFS embed.FS

// staticFS rebases rawStaticFS at its "static" subdirectory — see
// services/tooling/garage/static.go's doc comment for exactly why this
// rebase matters (a real bug found there: without it, "/static/htmx.min.js"
// 404s after http.StripPrefix, caught only by curling the running service).
var staticFS = func() fs.FS {
	sub, err := fs.Sub(rawStaticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}()
