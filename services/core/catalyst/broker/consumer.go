package broker

import (
	"context"

	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

type (
	Consumer interface {
		Consume(ctx context.Context, msg *acv1.Message, stream *connect.ServerStream[acv1.ConsumeResponse]) error
	}

	consumer struct {
		consumerRegistrationChan chan register
	}
)

func NewConsumer(consumerRegistrationChan chan register) Consumer {
	return &consumer{
		consumerRegistrationChan: consumerRegistrationChan,
	}

}

func (c *consumer) Consume(ctx context.Context, msg *acv1.Message, stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	// fling the consumer stream into the controller
	c.consumerRegistrationChan <- register{
		Message:      msg,
		ServerStream: stream,
	}

	return nil
}
