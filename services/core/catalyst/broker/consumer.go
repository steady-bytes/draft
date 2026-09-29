package broker

import (
	"context"

	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

type (
	Consumer interface {
		Consume(ctx context.Context, msg *acv1.CloudEvent, stream *connect.ServerStream[acv1.ConsumeResponse]) error
	}

	consumer struct {
		consumerRegistrationChan chan register
		state                    *atomicMap
	}
)

func NewConsumer(consumerRegistrationChan chan register, state *atomicMap) Consumer {
	return &consumer{
		consumerRegistrationChan: consumerRegistrationChan,
		state:                    state,
	}

}

// Consume registers msg's declared type with the controller's background goroutine (for topology
// bookkeeping — see controller.consume) and then delivers into stream itself, on this same
// goroutine, until ctx is cancelled or a Send fails. It runs on the request-handling goroutine the
// connect framework called this method from, and that framework will later run this RPC's
// deferred Close on that very goroutine's return — so delivery must happen here too, not in a
// separate goroutine racing with it over the same underlying HTTP/2 response writer.
func (c *consumer) Consume(ctx context.Context, msg *acv1.CloudEvent, stream *connect.ServerStream[acv1.ConsumeResponse]) error {
	subscribed := make(chan subscription, 1)
	c.consumerRegistrationChan <- register{
		ctx:          ctx,
		CloudEvent:   msg,
		ServerStream: stream,
		subscribed:   subscribed,
	}

	sub := <-subscribed
	defer c.state.Unsubscribe(sub.id)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-sub.ch:
			if err := stream.Send(&acv1.ConsumeResponse{Message: event}); err != nil {
				return err
			}
		}
	}
}
