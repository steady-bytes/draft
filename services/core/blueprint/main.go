package main

import (
	"embed"
	_ "embed"

	kv "github.com/steady-bytes/draft/blueprint/key_value"
	sd "github.com/steady-bytes/draft/blueprint/service_discovery"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
	"github.com/steady-bytes/draft/pkg/repositories/badger"
	"github.com/steady-bytes/draft/pkg/secrets/vault"
)

//go:embed web-client/dist/index.html
//go:embed web-client/dist/main.js
var files embed.FS

func main() {
	var (
		keyValueRepo       = badger.New()
		keyValueController = kv.NewController(keyValueRepo)
		keyValueRPC        = kv.New(keyValueController)
		secretStore        = vault.New("")

		serviceDiscoveryController = sd.NewController(keyValueController)
		serviceDiscoveryRPC        = sd.New(serviceDiscoveryController)
	)

	defer chassis.New(zerolog.New()).
		WithRepository(keyValueRepo).
		WithConsensus(chassis.Raft, keyValueController).
		WithRPCHandler(keyValueRPC).
		WithRPCHandler(serviceDiscoveryRPC).
		WithSecretStore(secretStore).
		WithClientApplication(files).
		Start()
}
