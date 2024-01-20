package main

import (
	kv "github.com/steady-bytes/draft/blueprint/key_value"
	sd "github.com/steady-bytes/draft/blueprint/service_discovery"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"google.golang.org/protobuf/proto"
)

const (
	NAME = "blueprint"
)

func main() {
	var (
		keyValueRepo       = kv.NewRepo[proto.Message]()
		keyValueController = kv.NewController(keyValueRepo)
		keyValueRPC        = kv.New(keyValueController)

		serviceDiscoveryController = sd.NewController(keyValueController)
		serviceDiscoveryRPC        = sd.New(serviceDiscoveryController)
	)

	defer draft.New(NAME, "").
		WithRepo(draft.Badger, keyValueRepo).
		WithConsensus(draft.Raft, keyValueController).
		WithRPCHandler(keyValueRPC).
		WithRPCHandler(serviceDiscoveryRPC).
		UseSecretStore(keyValueController).
		Start()
}
