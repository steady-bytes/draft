package broker

import (
	"context"
	"errors"
	"sync"

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
	producerPattern, producerHandler := acConnect.NewProducerHandler(h)
	server.AddHandler(producerPattern, producerHandler, true)

	// // regiser the handler for the producer service
	consumerPattern, consumerHandler := acConnect.NewConsumerHandler(h)
	server.AddHandler(consumerPattern, consumerHandler, true)
}

// Consume accepts a request containing a `Message` type to subscribe to
// to keep the connection open a `sync.WaitGroup` is created and the response
// stream, and message are passed to the `broker.Consume`
// Since the server stream can only return an error to close the connection
// the `wg.Done()` method is called after the error is logged closing the
// server connection with the client
func (h *rpc) Consume(ctx context.Context, req *connect.Request[acv1.ConsumeRequest], stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	h.logger.Info("consume request")

	var (
		msg = req.Msg.GetMessage()
		wg  sync.WaitGroup
	)

	wg.Add(1)
	if err := h.controller.Consume(ctx, msg, stream); err != nil {
		h.logger.Error(err.Error())
		wg.Done()
		return err
	}

	wg.Wait()

	return errors.New("closed")
}

func (h *rpc) Produce(ctx context.Context, inputStream *connect.BidiStream[acv1.ProduceRequest, acv1.ProduceResponse]) error {
	h.logger.Info("produce request")
	return h.controller.Produce(ctx, inputStream)
}
