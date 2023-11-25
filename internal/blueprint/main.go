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
		WithRepo(draft.Badger, model).
		WithConsensus(draft.Raft).
		// WithRPCHandler(raftHandler).
		Start()
}
