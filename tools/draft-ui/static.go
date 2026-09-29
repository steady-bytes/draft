package draftui

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
)

// StaticPrefix is where Static() is mounted. Templates reference assets through it (see Asset).
const StaticPrefix = "/static/draft/"

type staticFile struct {
	data        []byte
	etag        string
	contentType string
}

var (
	staticOnce  sync.Once
	staticFiles map[string]staticFile
)

// Public name -> path inside dist/. htmx is vendored for the Go services only.
var staticSources = map[string]string{
	"draft.css":                        "dist/draft.css",
	"fonts.css":                        "dist/fonts.css",
	"draft.js":                         "dist/draft.js",
	"boot.js":                          "dist/boot.js",
	"htmx.min.js":                      "dist/vendor/htmx.min.js",
	"fonts/inter-latin.woff2":          "dist/fonts/inter-latin.woff2",
	"fonts/jetbrains-mono-latin.woff2": "dist/fonts/jetbrains-mono-latin.woff2",
}

func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

func loadStatic() {
	staticFiles = make(map[string]staticFile, len(staticSources))
	for name, src := range staticSources {
		data, err := fs.ReadFile(assetsFS, src)
		if err != nil {
			// The embed directive names every source, so this cannot happen at runtime.
			panic("draftui: missing embedded asset " + src + ": " + err.Error())
		}
		sum := sha256.Sum256(data)
		staticFiles[name] = staticFile{
			data:        data,
			etag:        `"` + hex.EncodeToString(sum[:8]) + `"`,
			contentType: contentTypeFor(name),
		}
	}
}

// Asset returns the URL of an embedded asset, with a content hash so browsers refetch exactly
// when the file changes: Asset("draft.css") = "/static/draft/draft.css?v=1a2b3c4d".
func Asset(name string) string {
	staticOnce.Do(loadStatic)
	f, ok := staticFiles[name]
	if !ok {
		return StaticPrefix + name
	}
	return StaticPrefix + name + "?v=" + strings.Trim(f.etag, `"`)[:8]
}

// Static serves the embedded assets. Mount it at StaticPrefix:
//
//	mux.Handle("GET "+draftui.StaticPrefix, draftui.Static())
//
// Responses carry a content-hash ETag and `Cache-Control: no-cache`, so a request is always
// revalidated (cheap, answered with 304) and a rebuilt binary is never served stale.
func Static() http.Handler {
	staticOnce.Do(loadStatic)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, StaticPrefix)
		f, ok := staticFiles[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", f.contentType)
		h.Set("ETag", f.etag)
		h.Set("Cache-Control", "no-cache")
		if match := r.Header.Get("If-None-Match"); match == f.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write(f.data) //nolint:errcheck // the client went away
	})
}
