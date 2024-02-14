package chassis

import (
	"embed"
	"net/http"
)

func (c *Runtime) withClientApplication(e embed.FS) {
	// Init and store the http multiplexer
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	c.mux.Handle("/", http.StripPrefix("/", http.FileServer(http.FS(e))))

	c.isHTTP = true
}
