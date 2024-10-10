package consumer

import (
	pdConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	Rpc interface {
		chassis.RPCRegistrar
		pdConnect.ConsumerServiceHandler
	}

	rpc struct {
		controller Controller
		logger     chassis.Logger
	}
)

func NewRPC(logger chassis.Logger, controller Controller) Rpc {
	return &rpc{
		controller: controller,
		logger:     logger,
	}
}

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := pdConnect.NewConsumerServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}
