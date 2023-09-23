package draft_runtime

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
	RegisterBroker(interface{}) error
}

func (c *Commet) withBroker(registrar BrokerPluginRegistrar) {
	// if nats is the desired broker then connect to it
	if registrar.GetBrokerType() == Nats {
		nc, err := nats.Connect(nats.DefaultURL)
		if err != nil {
			panic(err)
		}

		registrar.RegisterBroker(nc)

		// store off a reference to the broker's connection
		c.nats = nc
	}
}
