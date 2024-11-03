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
		// Store the routine client connection
		m map[string][]*connect.ServerStream[acv1.ConsumeResponse]
		// yield the client connection to a thread, and then send events to it
		n map[string]chan *acv1.Message
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
		n:  make(map[string]chan *acv1.Message),
	}
}

func (am *atomicMap) Insert(key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	// TODO: Figure out how to start with a read lock?
	am.mu.RLock()
	defer am.mu.RUnlock()
	list, ok := am.m[key]
	if !ok {
		am.mu.RUnlock()
		am.mu.Lock()
		defer am.mu.Unlock()
		var list []*connect.ServerStream[acv1.ConsumeResponse]
		list = append(list, resStream)
		am.m[key] = list
	} else {
		list = append(list, resStream)
		am.m[key] = list
	}
}

func (am *atomicMap) Broker(key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	ch, found := am.n[key]
	if !found {
		// create the channel to add to map
		ch := make(chan *acv1.Message)

		// store channel in map for future connections
		am.mu.RUnlock()
		am.mu.Lock()
		am.n[key] = ch
		am.mu.Unlock()

		// now start a new routine and keep it open as long as the `ch` channel has connected clients
		go send(ch, resStream)
	} else {
		// the channel is already made and shared with other consumers, and producers so we can just use `ch`
		go send(ch, resStream)
	}
}

func send(ch chan *acv1.Message, stream *connect.ServerStream[acv1.ConsumeResponse]) {
	// when the channel receives a message send to the stream the client is holding onto
	for {
		m := <-ch
		msg := &acv1.ConsumeResponse{
			Message: m,
		}
		stream.Send(msg)
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

// consume - Will create a hash of the message domain, and typeUrl then save the msg.ServerStream to an `atomicMap`
// that can be used to `Broadcast` messages to when a message is produced. Con's to this approach are a `RWMutex`
// has to be used to `Broadcast` the message so the connected stream
// func (c *controller) consume(reg chan register) {
// 	for {
// 		msg := <-reg
// 		fmt.Print("Receive a request to setup a consumer", msg)

// 		// make hash of <domain><msg.Type.String>
// 		key := c.hash(msg.GetDomain(), msg.GetKind().GetTypeUrl())

// 		// use hash as key if the hash does not exist, then create a slice of connections
// 		// and append the connection to the slice
//		c.state.Insert(key, msg.ServerStream)
// 	}
// }

// consume - Will create a hash of the message domain, and typeUrl to use as a key to a tx, and rx sides of a channel
// the `tx` or transmitter will be used when a producer produces an event to send the event to each client that is consuming
// events of the domain, and typeUrl.
func (c *controller) consume(registerChan chan register) {
	for {
		// create a shared channel that will receive any kind of message of that domain, and typeUrl
		// add the receiver to a go routine that will keep the `ServerStream` open and send any messages
		// received up to the client connect.

		msg := <-registerChan
		key := c.hash(msg.GetDomain(), msg.GetKind().GetTypeUrl())
		c.state.Broker(key, msg.ServerStream)
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
