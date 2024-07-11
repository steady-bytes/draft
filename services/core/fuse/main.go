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

	// start the chassis
	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "fuse",
		}).
		WithRPCHandler(controlPlane).
		Start()
}
