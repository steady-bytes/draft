package queirer

import (
	"errors"
	"fmt"

	nats "github.com/nats-io/nats.go"
)

type queirerPublisher struct {
	broker *nats.Conn
}

func NewEventStoreProducer() *queirerPublisher {
	return &queirerPublisher{
		broker: nil,
	}
}

func (p *queirerPublisher) Publish(topic string, message []byte) error {
	if topic == "" {
		fmt.Println("error")
		return errors.New("topic can't be empty")
	}

	if err := p.broker.Publish(topic, message); err != nil {
		fmt.Println("error: ", err)
		return err
	}

	return nil
}
