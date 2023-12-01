package main

import (
	c "github.com/steady-bytes/draft/blueprint/controller"
	h "github.com/steady-bytes/draft/blueprint/handler"
	m "github.com/steady-bytes/draft/blueprint/model"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

func main() {
	var (
		mdl = m.New()
		ctr = c.New(mdl)
		hnd = h.New(ctr)
	)

	defer draft.New("blueprint", "").
		WithRepo(draft.Badger, mdl).
		WithConsensus(draft.Raft, ctr).
		WithRPCHandler(hnd).
		Start()
}
