package native

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStatusRecorder_CapturesStatus confirms the wrapper reports the status
// code a handler actually wrote, and defaults to 200 when WriteHeader is
// never called explicitly (matching net/http's own default).
func TestStatusRecorder_CapturesStatus(t *testing.T) {
	t.Run("explicit status", func(t *testing.T) {
		w := httptest.NewRecorder()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		rec.WriteHeader(http.StatusTeapot)
		if rec.status != http.StatusTeapot {
			t.Errorf("status = %d, want %d", rec.status, http.StatusTeapot)
		}
	})

	t.Run("default when WriteHeader never called", func(t *testing.T) {
		w := httptest.NewRecorder()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		rec.Write([]byte("ok")) //nolint:errcheck
		if rec.status != http.StatusOK {
			t.Errorf("status = %d, want default %d", rec.status, http.StatusOK)
		}
	})
}

// hijackableRecorder is an httptest.ResponseRecorder that also implements
// http.Hijacker, since httptest.NewRecorder's own type does not.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	server, _ := net.Pipe()
	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}

// TestStatusRecorder_ForwardsHijack is the regression test for the real bug
// this file's design fixed relative to the implementation plan's original
// sketch: wrapping a ResponseWriter to capture a status code must not
// silently break httputil.ReverseProxy's WebSocket/Upgrade support, which
// depends on type-asserting the ResponseWriter to http.Hijacker.
func TestStatusRecorder_ForwardsHijack(t *testing.T) {
	underlying := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	hijacker, ok := http.ResponseWriter(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder does not implement http.Hijacker -- would break WebSocket proxying through it")
	}
	if _, _, err := hijacker.Hijack(); err != nil {
		t.Fatalf("Hijack() returned an error: %v", err)
	}
	if !underlying.hijacked {
		t.Error("Hijack was not forwarded to the underlying ResponseWriter")
	}
}

func TestStatusRecorder_HijackErrorsWhenUnderlyingCannot(t *testing.T) {
	// httptest.NewRecorder()'s concrete type does not implement http.Hijacker.
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	hijacker, ok := http.ResponseWriter(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder must always implement http.Hijacker (even if it errors at call time), or type assertions against it in a real ReverseProxy would fail differently than against a real connection's ResponseWriter")
	}
	if _, _, err := hijacker.Hijack(); err == nil {
		t.Error("expected an error hijacking a non-Hijacker underlying ResponseWriter")
	}
}
