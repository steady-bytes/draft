package service

// import (
// 	"errors"
// 	"fmt"

// 	nats "github.com/nats-io/nats.go"
// )

// type eventStoreMessagePublisher struct {
// 	broker *nats.Conn
// }

// type MessagePublisher interface {
// 	Publish(topic string, message []byte) error
// }

// func NewMessagePublisher() *eventStoreMessagePublisher {
// 	return &eventStoreMessagePublisher{
// 		broker: nil,
// 	}
// }

// func (p *eventStoreMessagePublisher) Publish(topic string, message []byte) error {
// 	if topic == "" {
// 		fmt.Println("error")
// 		return errors.New("topic can't be empty")
// 	}

// 	if err := p.broker.Publish(topic, message); err != nil {
// 		fmt.Println("error: ", err)
// 		return err
// 	}

// 	return nil
// }
