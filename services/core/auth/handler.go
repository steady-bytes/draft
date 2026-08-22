package main

import (
	"net/http"

	"github.com/steady-bytes/draft/pkg/chassis"
)

// checkHandler implements the Envoy ext_authz HTTP check endpoint.
// Envoy sends every inbound request through here before forwarding it upstream.
// The handler either bypasses (always allow) or delegates to Authentik.
type checkHandler struct {
	cfg       authConfig
	fwd       *forwarder
	logger    chassis.Logger
}

func newCheckHandler(cfg authConfig, logger chassis.Logger) *checkHandler {
	var fwd *forwarder
	if cfg.mode == ModeAuthentik {
		fwd = newForwarder(cfg.authentikURL)
	}
	return &checkHandler{cfg: cfg, fwd: fwd, logger: logger}
}

// ServeHTTP handles all paths — Envoy sends the original request path as-is.
func (h *checkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch h.cfg.mode {
	case ModeBypass:
		// No identity provider; allow everything. Useful for local dev.
		w.WriteHeader(http.StatusOK)
	case ModeAuthentik:
		h.fwd.check(w, r)
	default:
		h.logger.WithField("mode", h.cfg.mode).Error("unknown auth mode — denying request")
		http.Error(w, "misconfigured auth service", http.StatusInternalServerError)
	}
}
