package main

import (
	"embed"

	kv "github.com/steady-bytes/draft/services/core/blueprint/key_value"
	sd "github.com/steady-bytes/draft/services/core/blueprint/service_discovery"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

//go:embed web-client/target/dx/blueprint-pwa/release/web
var files embed.FS

func main() {

	var (
		logger             = zerolog.New()
		keyValueModel      = kv.NewModel()
		keyValueController = kv.NewController(keyValueModel)
		keyValueRPC        = kv.NewRPC(logger, keyValueController)
	)

	c := chassis.New(logger).
		WithRepository(keyValueModel).
		WithConsensus(chassis.Raft, keyValueController)

	// initialize service discovery components here since the controller requires the RaftController from the chassis
	var (
		serviceDiscoveryController = sd.NewController(keyValueController, c.RaftController)
		serviceDiscoveryRPC        = sd.NewRPC(logger, serviceDiscoveryController)
	)

	c.WithRPCHandler(keyValueRPC).
		WithRPCHandler(serviceDiscoveryRPC).
		WithClientApplication(files)

	defer c.Start()
}
