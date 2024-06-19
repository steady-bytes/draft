package chassis

import (
	"net"
	"net/http"

	sdv1Cnt "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"
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

	// registry client
	blueprintClient sdv1Cnt.ServiceDiscoveryServiceClient
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
