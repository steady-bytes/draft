package main

import (
	"context"

	"github.com/steady-bytes/draft/pkg/chassis"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
	"github.com/steady-bytes/draft/services/examples/crud/service"
)

func main() {
	chassis.NewMetricsReporter().Start()

	var (
		logger = chassis.NewOTelLogger()
		db     = bun.New("")
		model  = service.NewModel(db)
	)

	runtime := chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "examples",
		}).
		WithRepository(db).
		WithRPCHandler(service.NewHandler(logger, model)).
		WithRoute(&ntv1.Route{
			Match: &ntv1.RouteMatch{
				Prefix: "/examples.crud.v1.CrudService/",
			},
		})

	if err := service.CreateSchema(context.Background(), db); err != nil {
		logger.WithError(err).Fatal("failed to create crud schema")
	}

	defer runtime.Start()
}
