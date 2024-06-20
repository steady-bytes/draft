package main

import (
	"embed"

	kv "github.com/steady-bytes/draft/services/core/blueprint/key_value"
	sd "github.com/steady-bytes/draft/services/core/blueprint/service_discovery"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

//go:embed web-client/dist/index.html
//go:embed web-client/dist/main.js
var files embed.FS

func main() {
	var (
		keyValueModel      = kv.NewModel()
		keyValueController = kv.NewController(keyValueModel)
		keyValueRPC        = kv.NewRpc(keyValueController)

		serviceDiscoveryController = sd.NewController(keyValueController)
		serviceDiscoveryRPC        = sd.New(serviceDiscoveryController)
	)

	defer chassis.New(zerolog.New()).
		WithRepository(keyValueModel).
		WithConsensus(chassis.Raft, keyValueController).
		WithRPCHandler(keyValueRPC).
		WithRPCHandler(serviceDiscoveryRPC).
		WithClientApplication(files).
		Start()
}
