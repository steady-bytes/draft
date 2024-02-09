package main

import (
	kv "github.com/steady-bytes/draft/blueprint/key_value"
	sd "github.com/steady-bytes/draft/blueprint/service_discovery"

	draft "github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/repositories/badger"
	"github.com/steady-bytes/draft/pkg/secrets/vault"
)

func main() {
	var (
		keyValueRepo       = badger.New()
		keyValueController = kv.NewController(keyValueRepo)
		keyValueRPC        = kv.New(keyValueController)
		secretStore        = vault.New("")

		serviceDiscoveryController = sd.NewController(keyValueController)
		serviceDiscoveryRPC        = sd.New(serviceDiscoveryController)
	)

	defer draft.New().
		WithRepository(keyValueRepo).
		WithConsensus(draft.Raft, keyValueController).
		WithRPCHandler(keyValueRPC).
		WithRPCHandler(serviceDiscoveryRPC).
		WithSecretStore(secretStore).
		Start()
}
