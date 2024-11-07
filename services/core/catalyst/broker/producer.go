package broker

import (
	"context"
	"errors"
	"fmt"
	"io"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"

	"connectrpc.com/connect"
)

type (
	Producer interface {
		Produce(ctx context.Context, inputStream *connect.BidiStream[acv1.ProduceRequest, acv1.ProduceResponse]) error
	}

	producer struct {
		producerChan chan *acv1.CloudEvent
	}
)

func NewProducer(produceChan chan *acv1.CloudEvent) Producer {
	return &producer{
		producerChan: produceChan,
	}
}

// Accepts an incomming bidirectional stream to keep open and push incomming
// messages into the broker when a message is `produce`'ed
func (p *producer) Produce(ctx context.Context, inputStream *connect.BidiStream[acv1.ProduceRequest, acv1.ProduceResponse]) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		request, err := inputStream.Receive()
		if err != nil && errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("receive request: %w", err)
		}

		p.producerChan <- request.GetMessage()
	}
}
