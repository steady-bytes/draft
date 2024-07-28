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
			// This is the host ip (IPv4) that the cluster will be forwarding the traffic to
			// in my case I am using the host machine's IP address of my dev box.
			Host: "10.0.0.108",
			Port: 8080,
			Match: &ntv1.RouteMatch{
				Prefix: "/",
			},
		}).
		Start()
}
