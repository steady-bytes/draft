package handler

import (
	c "github.com/steady-bytes/draft/internal/host/controller"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	hwv1 "github.com/steady-bytes/draft/api"
)

type (
	TestRPCHandler interface {
		draft.RPCRegistrar
		*hwv1.HelloworldServer
	}

	testRPCHandler struct {
		testCtrl c.TestController
	}
)
