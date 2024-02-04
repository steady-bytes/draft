package chassis

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

type BrokerRegistrar interface {
	// SetBrokerType - gives the running service an opportunity to
	SetBrokerType() BrokerType
	RegisterBroker(interface{}) error
}

func (c *Runtime) withBroker(registrar BrokerRegistrar) {
	// if nats is the desired broker then connect to it
	if registrar.SetBrokerType() == Nats {
		nc, err := nats.Connect(nats.DefaultURL)
		if err != nil {
			panic(err)
		}

		registrar.RegisterBroker(nc)

		// store off a reference to the broker's connection
		c.nats = nc
	}
}
