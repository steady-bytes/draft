package main

import (
	"embed"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

//go:embed web-client/dist/index.html
var files embed.FS

func main() {
	var (
		logger = zerolog.New()
	)

	defer chassis.New(logger).
		WithClientApplication(files).
		WithRoute(&ntv1.Route{
			Name: "file_host",
			Host: "0.0.0.0",
			Port: 8080,
			Match: &ntv1.RouteMatch{
				Prefix: "/",
			},
		}).
		Start()
}
