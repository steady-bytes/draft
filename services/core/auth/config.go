package main

import "github.com/steady-bytes/draft/pkg/chassis"

const (
	// ModeBypass accepts every check request without contacting an identity provider.
	// Safe for local development when Authentik is not running.
	ModeBypass   = "bypass"
	ModeAuthentik = "authentik"

	configModeKey      = "auth.mode"
	configAuthentikURL = "auth.authentik.url"
)

type authConfig struct {
	mode        string
	authentikURL string
}

func loadAuthConfig() authConfig {
	cfg := chassis.GetConfig()
	mode := cfg.GetString(configModeKey)
	if mode == "" {
		mode = ModeBypass
	}
	return authConfig{
		mode:        mode,
		authentikURL: cfg.GetString(configAuthentikURL),
	}
}
