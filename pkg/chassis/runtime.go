package chassis

import (
	"net"
	"net/http"
	"sync"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	sdv1Cnt "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"
)

type Runtime struct {
	config                    Config
	logger                    Logger
	brokers                   []Broker
	repositories              []Repository
	secretStores              []SecretStore
	isRPC                     bool
	noMux                     bool
	rpcReflectionServiceNames []string
	rpcServiceNames           []string
	mux                       *http.ServeMux
	consensusKind             ConsensusKind
	raftAdvertiseAddress      *net.TCPAddr
	RaftController            RaftController
	onStart                   []func()
	blueprintClient           sdv1Cnt.ServiceDiscoveryServiceClient
	blueprintCluster          *BlueprintCluster
}

type BlueprintCluster struct {
	sync.Mutex
	Nodes []*sdv1.Node
}

func New(logger Logger) *Runtime {
	if logger == nil {
		panic("logger cannot be nil")
	}

	rt := &Runtime{
		config: LoadConfig(),
		blueprintCluster: &BlueprintCluster{
			Mutex: sync.Mutex{},
			Nodes: []*sdv1.Node{},
		},
	}

	logger.Start(rt.config)
	rt.logger = logger
	return rt
}
