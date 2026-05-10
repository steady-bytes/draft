package main

import (
	broker "github.com/steady-bytes/draft/services/core/catalyst/broker"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	logger := zerolog.New()

	cfg := chassis.GetConfig()

	var chCfg broker.ClickHouseConfig
	if err := cfg.UnmarshalKey("clickhouse", &chCfg); err != nil {
		logger.WithField("error", err.Error()).Error("failed to read clickhouse config")
	}

	var storer broker.Storer
	if chCfg.Enabled {
		s, err := broker.NewClickHouseStore(chCfg)
		if err != nil {
			logger.WithField("error", err.Error()).Error("failed to connect to clickhouse — falling back to noop store")
			storer = broker.NewNoopStore()
		} else {
			storer = s
		}
	} else {
		storer = broker.NewNoopStore()
	}

	cnt := broker.NewController(logger, storer)
	rpc := broker.NewRPC(logger, cnt)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "core",
		}).
		WithRPCHandler(rpc).
		Start()
}
