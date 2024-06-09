package main

import (
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	defer chassis.New(zerolog.New()).
		Register(chassis.RegistrationOptions{
			Namespace: "fuse",
		}).
		Start()
}
