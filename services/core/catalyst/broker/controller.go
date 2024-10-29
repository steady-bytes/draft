package broker

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"connectrpc.com/connect"
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

		state *atomicMap
	}
	atomicMap struct {
		mu sync.RWMutex
		m  map[string][]*connect.ServerStream[acv1.ConsumeResponse]
	}

	register struct {
		*acv1.Message
		*connect.ServerStream[acv1.ConsumeResponse]
	}
)

func newState() *atomicMap {
	return &atomicMap{
		mu: sync.RWMutex{},
		m:  make(map[string][]*connect.ServerStream[acv1.ConsumeResponse]),
	}
}

func (am *atomicMap) Insert(key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	am.mu.Lock()
	defer am.mu.Unlock()
	list, ok := am.m[key]
	if !ok {
		var list []*connect.ServerStream[acv1.ConsumeResponse]
		list = append(list, resStream)

		am.m[key] = list
	} else {
		list = append(list, resStream)
		am.m[key] = list
	}
}

func (am *atomicMap) Broadcast(key string, msg *acv1.Message) {
	am.mu.Lock()
	defer am.mu.Unlock()

	list, ok := am.m[key]
	if ok {
		res := &acv1.ConsumeResponse{
			Message: msg,
		}

		for _, v := range list {
			v.Send(res)
		}
	} else {
		// we don't have any consumers that will listen to the message

		// TODO: We might consider a dead letter queue
	}
}

func NewController() Controller {
	var (
		producerMsgChan          chan *acv1.Message
		consumerRegistrationChan = make(chan register)
	)

	ctr := &controller{
		NewProducer(producerMsgChan),
		NewConsumer(consumerRegistrationChan),
		newState(),
	}

	// TODO: This could contain more configuration. Like maybe reading the number
	// 		 of cpu cores to spread the works over?

	go ctr.produce(producerMsgChan)
	go ctr.consume(consumerRegistrationChan)

	return ctr
}

func (c *controller) produce(producerMsgChan chan *acv1.Message) {
	for {
		msg := <-producerMsgChan
		fmt.Print("Received msg from producer: ", msg)

		// make hash of <domain><msg.Type.String>
		key := c.hash(msg.GetDomain(), msg.GetKind().GetTypeUrl())

		// do I save to blueprint?
		// do I push into a queue for processing

		// send the received `Message` to all `Consumers` for the same key
		c.state.Broadcast(key, msg)
	}
}

func (c *controller) consume(reg chan register) {
	for {
		msg := <-reg
		fmt.Print("Receive a request to setup a consumer", msg)

		// make hash of <domain><msg.Type.String>
		key := c.hash(msg.GetDomain(), msg.GetKind().GetTypeUrl())

		// use hash as key if the hash does not exist, then create a slice of connections
		// and append the connection to the slice
		c.state.Insert(key, msg.ServerStream)
	}
}

// hash to calculate the same key for two strings
func (c *controller) hash(domain, msgKindName string) string {
	var (
		domainHash      = sha256.New()
		msgKindNameHash = sha256.New()
	)

	domainHash.Write([]byte(domain))
	msgKindNameHash.Write([]byte(msgKindName))
	out := append(domainHash.Sum(nil), msgKindNameHash.Sum(nil)...)

	return fmt.Sprintf("%X", []byte(out))
}
