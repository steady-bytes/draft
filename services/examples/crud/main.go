package main

import (
	draft "github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
	"github.com/steady-bytes/draft/services/examples/crud/service"
)

func main() {

	var (
		db    = bun.New("")
		model = service.NewModel(db)
	)

	defer draft.New(zerolog.New()).
		WithRepository(db).
		WithRPCHandler(service.NewHandler(model)).
		Start()
}
