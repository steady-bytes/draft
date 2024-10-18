package broker

import (
	"context"
	"errors"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	// Given the producer, and consumer will have to share the same in memory data structure to
	// send message to consumers when received from a producer. It's worth keeping the rpc services
	// defined seperatly but implement using the same handler layer
	Rpc interface {
		chassis.RPCRegistrar
		acConnect.ConsumerHandler
		acConnect.ProducerHandler
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
	// regiser the handler for the consumer service
	producerPattern, producerHandler := acConnect.NewConsumerHandler(h)
	server.AddHandler(producerPattern, producerHandler, true)
	// regiser the handler for the producer service
	consumerPattern, consumerHandler := acConnect.NewConsumerHandler(h)
	server.AddHandler(consumerPattern, consumerHandler, true)
}

func (h *rpc) Consume(ctx context.Context, request *connect.Request[acv1.ConsumeRequest], stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	return errors.New("not implemented")
}

func (h *rpc) Produce(ctx context.Context, inputStream *connect.BidiStream[acv1.ProduceRequest, acv1.ProduceRequest]) error {
	return errors.New("not implemented")
}
