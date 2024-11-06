package broker

import (
	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	Controller interface {
		Consumer
		Producer
	}

	controller struct {
		Producer
		Consumer

		logger chassis.Logger

		state *atomicMap
	}

	register struct {
		*acv1.CloudEvent
		*connect.ServerStream[acv1.ConsumeResponse]
	}
)

func NewController(logger chassis.Logger) Controller {
	var (
		producerMsgChan          = make(chan *acv1.CloudEvent)
		consumerRegistrationChan = make(chan register)
	)

	ctr := &controller{
		NewProducer(producerMsgChan),
		NewConsumer(consumerRegistrationChan),
		logger,
		newAtomicMap(),
	}

	// TODO: This could contain more configuration. Like maybe reading the number
	// 		 of cpu cores to spread the works over?

	go ctr.produce(producerMsgChan)
	go ctr.consume(consumerRegistrationChan)

	return ctr
}

const (
	LOG_KEY_TO_CH = "key to channel"
)

func (c *controller) produce(producerMsgChan chan *acv1.CloudEvent) {
	for {
		msg := <-producerMsgChan
		c.logger.WithField("msg: ", msg).Info("produce massage received")

		// make hash of <domain><msg.Type.String>
		key := c.state.hash(string(msg.ProtoReflect().Descriptor().FullName()))

		// do I save to blueprint?
		// - default config is to be durable
		// - the producer can also add configuration to say not to store

		// send the received `Message` to all `Consumers` for the same key
		c.logger.WithField("key", key).Info(LOG_KEY_TO_CH)
		c.state.Broadcast(key, msg)
	}
}

// consume - Will create a hash of the message domain, and typeUrl then save the msg.ServerStream to `atomicMap.m`
// that can be used to `Broadcast` messages to when a message is produced. Con's to this approach are a `RWMutex`
// has to be used to `Broadcast` the message so the connected stream.
// func (c *controller) consume(reg chan register) {
// 	for {
// 		msg := <-reg
// 		fmt.Print("Receive a request to setup a consumer", msg)

// 		// make hash of <domain><msg.Type.String>
// 		key := c.hash(msg.GetDomain(), msg.GetKind().GetTypeUrl())

// 		// use hash as key if the hash does not exist, then create a slice of connections
// 		// and append the connection to the slice
// 		c.state.Insert(key, msg.ServerStream)
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
		c.logger.WithField("msg", msg).Info("consume channel registration")

		key := c.state.hash(string(msg.ProtoReflect().Descriptor().FullName()))

		// key := c.state.hash(msg.GetDomain(), msg.GetKind().GetTypeUrl())
		c.logger.WithField("key", key).Info(LOG_KEY_TO_CH)
		c.state.Broker(key, msg.ServerStream)
	}
}
