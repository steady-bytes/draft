package main

import (
	"net/http"

	draft "github.com/steady-bytes/draft/pkg/chassis"
)

func main() {
	h := &Healthz{
		status: "healthy",
	}

	defer draft.New("healthz", "").
		WithHTTPHandler(draft.Std, h).
		Start()
}

type Healthz struct {
	status string
}

func (h *Healthz) RegisterHTTP(router interface{}) error {
	var (
		mux   *http.ServeMux
		isMux bool
	)

	if router == nil {
		return draft.ErrEmptyHttpMultiplexer
	}

	if mux, isMux = router.(*http.ServeMux); !isMux {
		return draft.ErrMuxFailedTypecast
	}

	healthz := http.HandlerFunc(h.healthZ)

	mux.Handle("/healthz", healthz)

	return nil
}

func (h *Healthz) Health() string {
	return h.status
}

func (h *Healthz) healthZ(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(h.Health()))
}
