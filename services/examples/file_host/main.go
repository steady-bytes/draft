package main

import (
	"embed"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

//go:embed web-client/dist/index.html
var files embed.FS

func main() {
	var (
		logger = zerolog.New()
	)

	defer chassis.New(logger).
		WithClientApplication(files).
		Start()
}
