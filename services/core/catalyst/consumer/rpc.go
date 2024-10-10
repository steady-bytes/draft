package consumer

import (
	"context"
	"errors"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	Rpc interface {
		chassis.RPCRegistrar
		acConnect.ConsumerHandler
	}

	rpc struct {
		controller Controller
		logger     chassis.Logger
	}
)

func NewRPC(logger chassis.Logger, controller Controller) Rpc {
	return &rpc{
		logger:     logger,
		controller: controller,
	}
}

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := acConnect.NewConsumerHandler(h)
	server.AddHandler(pattern, handler, true)
}

func (h *rpc) Consume(ctx context.Context, request *connect.Request[acv1.ConsumeRequest], stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	return errors.New("not implemented")
}
