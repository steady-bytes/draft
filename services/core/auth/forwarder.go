package main

import (
	"io"
	"net/http"
)

// forwarder proxies an ext_authz check request to the Authentik embedded outpost.
// Envoy sends the original request headers to this service; we relay them to Authentik
// and return its decision (200 allow, 4xx deny) with any identity headers intact.
type forwarder struct {
	authentikCheckURL string
	client            *http.Client
}

func newForwarder(authentikBaseURL string) *forwarder {
	return &forwarder{
		// Authentik's embedded outpost ext_authz endpoint
		authentikCheckURL: authentikBaseURL + "/outpost.goauthentik.io/auth/envoy",
		client:            &http.Client{},
	}
}

func (f *forwarder) check(w http.ResponseWriter, r *http.Request) {
	outReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, f.authentikCheckURL, nil)
	if err != nil {
		http.Error(w, "failed to build auth request", http.StatusInternalServerError)
		return
	}

	// Copy original request headers so Authentik can read cookies, Authorization, etc.
	for key, vals := range r.Header {
		for _, v := range vals {
			outReq.Header.Add(key, v)
		}
	}

	resp, err := f.client.Do(outReq)
	if err != nil {
		http.Error(w, "auth service unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Relay identity headers (X-Authentik-*) so the upstream service can read them.
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
