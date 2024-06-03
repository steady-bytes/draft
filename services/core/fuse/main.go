package main

import (
	bp "github.com/steady-bytes/draft/services/core/blueprint/service_discovery/client"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	// start the registration process with blueprint
	var (
		_ = bp.NewClient()
	)

	defer chassis.New(zerolog.New()).Start()
}
