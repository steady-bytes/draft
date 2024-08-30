package chassis

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

const (
	webClientRoot = "web-client/dist"
)

// NOTE: the logic for serving a SPA is modified from the example here: https://github.com/gorilla/mux#serving-single-page-applications

// spaHandler implements the http.Handler interface, so we can use it
// to respond to HTTP requests. The path to the index file within the embedded
// file system is used to serve the SPA.
type spaHandler struct {
	indexPath  string
	fileSystem fs.FS
}

// ServeHTTP inspects the URL path to locate a file within the file system
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// clean and trim path (default to index)
	path := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if path == "" {
		path = h.indexPath
	}

	// check whether a file exists or is a directory at the given path
	f, err := h.fileSystem.Open(path)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFileFS(w, r, h.fileSystem, h.indexPath)
		return
	}
	if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) opening the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fi, err := f.Stat()
	if err != nil {
		// if we got an error stating the file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fi.IsDir() {
		// if path is a directory, serve index.html
		http.ServeFileFS(w, r, h.fileSystem, h.indexPath)
		return
	}

	// otherwise, serve without modification
	http.ServeFileFS(w, r, h.fileSystem, path)
}

func (c *Runtime) withClientApplication(e embed.FS) {
	// Init and store the http multiplexer
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	spa := spaHandler{
		indexPath:  "index.html",
		fileSystem: c.getFileSystem(e),
	}
	c.mux.Handle("/", spa)
}

func (c *Runtime) getFileSystem(e embed.FS) fs.FS {
	fsys, err := fs.Sub(e, webClientRoot)
	if err != nil {
		panic(err)
	}
	return fs.FS(fsys)
}
