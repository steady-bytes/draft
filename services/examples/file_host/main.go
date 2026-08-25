package main

import (
	"embed"

	"github.com/steady-bytes/draft/pkg/chassis"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

//go:embed web-client/dist/index.html
var files embed.FS

func main() {
	chassis.NewMetricsReporter().Start()
	var (
		logger = chassis.NewOTelLogger()
	)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "examples",
		}).
		WithClientApplication(files, "web-client/dist").
		WithRoute(&ntv1.Route{
			Match: &ntv1.RouteMatch{
				Prefix: "/",
			},
		}).
		Start()
}
