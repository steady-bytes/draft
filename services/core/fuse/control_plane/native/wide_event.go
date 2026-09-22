package native

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// statusRecorder captures the status code a ReverseProxy wrote, which
// httputil.ReverseProxy never exposes to its caller directly. Only used
// when a span is actually being recorded (see serveHTTP) -- wrapping the
// ResponseWriter has a real cost worth avoiding on the common path when
// WideEvent production is off.
//
// Forwards Hijack and Flush to the underlying ResponseWriter: without
// this, wrapping w here to capture a status code would silently break the
// WebSocket/Upgrade proxying httputil.ReverseProxy already provides (it
// hijacks the server-side connection directly, and a wrapper that only
// embeds http.ResponseWriter without forwarding http.Hijacker breaks that
// type assertion) -- a real regression this phase must not introduce for
// the sake of an attribute on a WideEvent.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
	}
	return h.Hijack()
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
