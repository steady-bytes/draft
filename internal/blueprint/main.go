package main

import (
	a "github.com/steady-bytes/draft/blueprint/api"
	c "github.com/steady-bytes/draft/blueprint/controller"
	r "github.com/steady-bytes/draft/blueprint/repo"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

func main() {
	var (
		repo = r.New[any]()
		ctr  = c.New(repo)
		api  = a.New(ctr)
	)

	defer draft.New("blueprint", "").
		WithRepo(draft.Badger, repo).
		WithConsensus(draft.Raft, ctr).
		WithRPCHandler(api).
		WithSecretStore(ctr).
		Start()
}
