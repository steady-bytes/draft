package chassis

import (
	"net"
	"net/http"
)

type Runtime struct {
	config Config
	logger Logger

	brokers      []Broker
	repositories []Repository
	secretStores []SecretStore

	isRPC                     bool
	rpcReflectionServiceNames []string
	mux                       *http.ServeMux

	consensusKind        ConsensusKind
	raftAdvertiseAddress *net.TCPAddr
}

func New(logger Logger) *Runtime {
	if logger == nil {
		panic("logger cannot be nil")
	}
	rt := &Runtime{
		config: LoadConfig(),
	}
	logger.Start(rt.config)
	rt.logger = logger
	return rt
}
