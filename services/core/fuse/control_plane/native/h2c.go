package native

import (
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// h2cHandler wraps handler so the same plaintext listener accepts both
// HTTP/1.1 and cleartext HTTP/2 (h2c) -- Envoy's HttpConnectionManager
// already does both on one listener without extra configuration; net/http
// needs this explicit wrapper for the h2c half, since http.Server only
// negotiates HTTP/2 over TLS (ALPN) on its own.
func h2cHandler(handler http.Handler) http.Handler {
	return h2c.NewHandler(handler, &http2.Server{})
}
