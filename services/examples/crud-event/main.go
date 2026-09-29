package main

import (
	"context"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
	"github.com/steady-bytes/draft/services/examples/crud-event/service"
)

func main() {
	chassis.NewMetricsReporter().Start()

	var (
		logger = chassis.NewOTelLogger()
		db     = bun.New("")
		model  = service.NewModel(db)
	)

	// eventsCtx lives for the whole process (cancelled on chassis.Closer(), the same shutdown
	// signal services/examples/producer/main.go's own Produce stream watches) — not scoped to any
	// one request, since the publisher's stream to Catalyst is opened once and reused by every
	// RPC handler call for as long as this service runs.
	eventsCtx, cancel := context.WithCancel(context.Background())
	go func() {
		<-chassis.Closer()
		cancel()
	}()

	cfg := chassis.GetConfig()
	catalystAddr := cfg.GetString("catalyst.address")
	if catalystAddr == "" {
		catalystAddr = "http://localhost:2220"
	}
	events := service.NewEventPublisher(eventsCtx, logger, catalystAddr, "/services/examples/crud-event")

	// No WithRoute: services/examples/crud already registers Fuse's ingress route for
	// examples.crud.v1.CrudService's own path prefix, and every RPC this service serves is that
	// exact same generated Connect service (proto paths are fixed by the .proto, not something a
	// second registration can rename) — a second WithRoute here would collide with crud's in
	// Fuse's routing table rather than living alongside it. crud-event is reachable directly on
	// its own bind_port (see config.yaml) for the same reason services/examples/producer, which
	// also registers no route, is.
	runtime := chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "examples",
		}).
		WithRepository(db).
		WithRPCHandler(service.NewHandler(logger, model, events))

	if err := service.CreateSchema(context.Background(), db); err != nil {
		logger.WithError(err).Fatal("failed to create crud-event schema")
	}

	defer runtime.Start()
}
