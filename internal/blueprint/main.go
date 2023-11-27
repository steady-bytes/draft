package main

import (
	m "github.com/steady-bytes/draft/blueprint/model"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

func main() {
	var (
		name = "blueprint"
	)

	model := m.NewKeyValueModel()

	defer draft.New(name, "").
		// setup the persistence layer
		// Since the consensus module is in draft. It might be worth moving
		WithRepo(draft.Badger, model).
		WithConsensus(draft.Raft, model).
		WithRPCHandler(model).
		Start()
}
