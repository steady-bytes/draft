package main

import (
	broker "github.com/steady-bytes/draft/services/core/catalyst/broker"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	var (
		logger = zerolog.New()

		cnt = broker.NewController()
		rpc = broker.NewRPC(logger, cnt)
	)

	defer chassis.New(logger).
		WithRPCHandler(rpc).
		Start()
}
