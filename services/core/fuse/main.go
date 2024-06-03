package main

import (
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	var ()

	// implement health checks service
	// maybe that's already in the chassis package

	// start the registration process with blueprint

	defer chassis.New(zerolog.New()).Start()
}
