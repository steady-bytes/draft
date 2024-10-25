package broker

import (
	"fmt"
	"sync"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

type (
	Controller interface {
		Consumer
		Producer
	}

	controller struct {
		Producer
		Consumer

		lock        sync.RWMutex
		connections map[string]string
	}
)

func NewController() Controller {
	producerMsgChan := make(chan acv1.Message)

	ctr := &controller{
		NewProducer(producerMsgChan),
		NewConsumer(),
		sync.RWMutex{},
		make(map[string]string),
	}

	ctr.start(producerMsgChan)

	return ctr
}

func (c *controller) start(producerMsgChan chan acv1.Message) {
	for {
		msg := <-producerMsgChan
		fmt.Print("Received msg from producer: ", msg)
	}
}
