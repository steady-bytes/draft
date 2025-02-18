package chassis

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

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

func (b *BlueprintCluster) NodeCount() int {
	return len(b.Nodes)
}

// remove the first item from the `Nodes` slice while returning the same value
func (b *BlueprintCluster) Pop() *sdv1.Node {
	if b.NodeCount() > 0 {
		n := b.Nodes[0]
		b.Lock()
		defer b.Unlock()
		b.Nodes = append(b.Nodes[:0], b.Nodes[1:]...)
		return n
	} else {
		return nil
	}
}

func New(logger Logger) *Runtime {
	// set up closer channel to handle graceful shutdown
	closer = make(chan os.Signal, 1)
	signal.Notify(closer, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

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
