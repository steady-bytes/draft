package main

import (
	"context"

	echov1 "github.com/steady-bytes/draft/api/examples/echo/v1"
	echov1Connect "github.com/steady-bytes/draft/api/examples/echo/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	"connectrpc.com/connect"
)

func main() {
	defer chassis.New(zerolog.New()).
		WithRPCHandler(&controller{}).
		Start()
}

type Rpc interface {
	chassis.RPCRegistrar
	echov1Connect.EchoServiceHandler
}

type controller struct{}

func (h *controller) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := echov1Connect.NewEchoServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

func (c *controller) Speak(ctx context.Context, req *connect.Request[echov1.SpeakRequest]) (*connect.Response[echov1.SpeakResponse], error) {
	return connect.NewResponse(&echov1.SpeakResponse{
		Output: req.Msg.Input,
	}), nil
}
