package commet

import "github.com/nats-io/nats.go"

// PUB/SUB
// broker

// BrokerType - tells the runtime what type of connection to make with a specific message
// broker. Currently `Nats` is the only inegration but this could be extended if system
// requirements change
type BrokerType int

const (
	NullBrokerType = iota
	Nats
)

// String - get the human readable value for `BrokerType`
func (pt BrokerType) String() string {
	return []string{"null", "nats"}[pt]
}

type BrokerPluginRegistrar interface {
	GetBrokerType() BrokerType
}

func (c *Commet) withBroker() {
	// if nats is the desired broker then connect to it
	if c.defaultPlugin.GetBrokerType() == Nats {
		nc, err := nats.Connect(nats.DefaultURL)
		if err != nil {
			panic(err)
		}

		// store off a reference to the broker's connection
		c.nats = nc
	}
}

// publisher

type PublisherPluginRegistrar interface {
	// RegisterProducer - registers a producer of events
	RegisterPublisher(interface{}) error
}

func (c *Commet) withPublisher() {
	// register the connection with the plugin that will consume connection, and produce messages
	if c.defaultPlugin.GetBrokerType() != NullBrokerType {

		/* if err := c.defaultPlugin.RegisterPublisher(c.nats); err != nil {
		 *   panic(err)
		 * } */

	}
}

// subscriber

type SubscriberPluginRegistrar interface {
	// register the connect with the plugin that will consumer the connection, listen for messages
	RegisterSubscriber(interface{}) error
}

func (c *Commet) withSubscriber() {
	// if err := c.defaultPlugin.RegisterSubscriber(c.nats); err != nil {
	//		panic(err)
	//	}
}
