package main

import (
	c "github.com/steady-bytes/draft/internal/host/controller"
	h "github.com/steady-bytes/draft/internal/host/handler"
	m "github.com/steady-bytes/draft/internal/host/model"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"google.golang.org/grpc"
)

func main() {
	var (
		model       m.TestModel
		ctrl        c.TestController
		testHandler h.TestHTTPHandler
	)

	model = m.NewTestModel()
	ctrl = c.New(model)
	testHandler = h.NewTestView(ctrl)

	defer draft.New("host").
		WithRepo(draft.PostgresBun, model).
		WithHTTPHandler(draft.Gin, testHandler).
		// WithRPCHandler(draft.Grpc, view).
		Start()
}

// Implementing the `draft.Plugin` interface so it can be run as a plugin to the draft Runtime
type gateway struct {
	draft.Default
}

// Constructor to build a plugin that can be used by the runtime
func New() draft.Default {
	return &gateway{}
}

func (g *gateway) RPC() *grpc.Server {
	// server := grpc.NewServer()
	// api.RegisterRegistryServer(server, g.service)

	return nil
}

func (g *gateway) BrokerType() draft.BrokerType {
	return draft.NullBrokerType
}
