package main

import (
	"context"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	echov1 "github.com/steady-bytes/draft/api/examples/echo/v1"
	echov1Connect "github.com/steady-bytes/draft/api/examples/echo/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

func main() {
	chassis.NewMetricsReporter().Start()
	var (
		logger = chassis.NewOTelLogger()
		ctrl   = &controller{
			logger: logger,
		}
	)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "examples",
		}).
		WithRPCHandler(ctrl).
		WithRoute(&ntv1.Route{
			Match: &ntv1.RouteMatch{
				Prefix: "/examples.echo.v1.EchoService/",
			},
		}).
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
	pattern, handler := echov1Connect.NewEchoServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

func (c *controller) Speak(ctx context.Context, req *connect.Request[echov1.SpeakRequest]) (*connect.Response[echov1.SpeakResponse], error) {
	c.logger.WithField("input", req.Msg.Input).Info("received request")
	return connect.NewResponse(&echov1.SpeakResponse{
		Output: req.Msg.Input,
	}), nil
}
