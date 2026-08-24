package main

import (
	"embed"
	"io/fs"
)

// rawStaticFS embeds static/htmx.min.js (vendored locally, per the design
// doc's "a single vendored/CDN-free local JS file is fine" allowance — see
// ui.go's route registration for where it's served from) into the compiled
// binary. Its root is the package directory, so the embedded path is
// "static/htmx.min.js", not "htmx.min.js".
//
//go:embed static/htmx.min.js
var rawStaticFS embed.FS

// staticFS rebases rawStaticFS at its "static" subdirectory, so
// staticFS's root *is* "htmx.min.js" — matching what ui.go's
// http.StripPrefix("/static/", ...) leaves behind after stripping a
// request's "/static/" prefix. Without this, a request for
// "/static/htmx.min.js" resolves (post-strip) to "htmx.min.js", which
// doesn't exist at rawStaticFS's root (only "static/htmx.min.js" does) —
// a 404 caught by curl-ing the running service, not by go vet/build.
var staticFS = func() fs.FS {
	sub, err := fs.Sub(rawStaticFS, "static")
	if err != nil {
		// Unreachable: "static" is a literal, compile-time-verified
		// subdirectory of the //go:embed above.
		panic(err)
	}
	return sub
}()
