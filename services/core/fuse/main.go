package main

import (
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	cp "github.com/steady-bytes/draft/fuse/control_plane"
)

func main() {
	var (
		logger       = zerolog.New()
		controlPlane = cp.New(logger)
	)
	defer chassis.New(zerolog.New()).
		Register(chassis.RegistrationOptions{
			Namespace: "fuse",
		}).
		WithRPCHandler(controlPlane).
		Start()
}
