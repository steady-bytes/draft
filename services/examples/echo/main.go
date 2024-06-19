package main

import (
	"context"

	echov1 "github.com/steady-bytes/draft/api/examples/echo/v1"
	echov1Connect "github.com/steady-bytes/draft/api/examples/echo/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	draft "github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	"connectrpc.com/connect"
)

func main() {
	defer draft.New(zerolog.New()).
		WithRPCHandler(&controller{}).
		Start()
}

type Rpc interface {
	chassis.RPCRegistrar
	echov1Connect.EchoServiceHandler
}

type controller struct {
	logger chassis.Logger
}

func (h *controller) RegisterRPC(server chassis.Rpcer) {
	server.EnableReflection(echov1Connect.EchoServiceName)
	server.AddHandler(echov1Connect.NewEchoServiceHandler(h))
	h.logger = server.Logger()
}

func (c *controller) Speak(ctx context.Context, req *connect.Request[echov1.SpeakRequest]) (*connect.Response[echov1.SpeakResponse], error) {
	return connect.NewResponse(&echov1.SpeakResponse{
		Output: req.Msg.Input,
	}), nil
}
