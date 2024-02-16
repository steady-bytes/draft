package chassis

import (
	"embed"
	"io/fs"
	"net/http"
)

func (c *Runtime) withClientApplication(e embed.FS) {
	// Init and store the http multiplexer
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	c.mux.Handle("/", http.FileServer(c.getFileSystem(e)))
}

func (c *Runtime) getFileSystem(e embed.FS) http.FileSystem {
	fsys, err := fs.Sub(e, "web-client/dist")
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}
