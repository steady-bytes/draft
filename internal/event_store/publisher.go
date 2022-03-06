package event_store

import (
	"errors"
	"fmt"

	nats "github.com/nats-io/nats.go"
)

type eventStorePublisher struct {
	broker *nats.Conn
}

func NewEventStorePublisher() *eventStorePublisher {
	return &eventStorePublisher{
		broker: nil,
	}
}

func (p *eventStorePublisher) Publish(topic string, message []byte) error {
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
