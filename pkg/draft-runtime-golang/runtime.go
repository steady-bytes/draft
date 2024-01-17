package draft_runtime_golang

import (
	"fmt"
	"net"
	"net/http"

	"github.com/dgraph-io/badger/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/nats-io/nats.go"
	"github.com/uptrace/bun"
)

type Runtime struct {
	config *Config
	nodeID string

	// repo options
	badger   *badger.DB
	gorm     *gorm.DB
	bun      *bun.DB
	repoKind RepoKind

	// network toggles
	isRPC                     bool
	rpcReflectionServiceNames []string
	isHTTP                    bool
	// multiplexer
	mux *http.ServeMux
	// router options
	gin      *gin.Engine
	httpKind HTTPKind

	nats *nats.Conn

	consensusKind        ConsensusKind
	raftAdvertiseAddress *net.TCPAddr

	plugin Default
}

func New(name, nodeID string) *Runtime {
	if nodeID == "" {
		nodeID = uuid.NewString()
	}

	rt := &Runtime{
		config: NewConfig(name),
		nodeID: nodeID,
		bun:    nil,
		isRPC:  false,
		isHTTP: false,
	}

	return rt
}

func (rt Runtime) NodeID() string {
	return rt.nodeID
}

func (rt Runtime) Title() string {
	return fmt.Sprintf("%s_%s", rt.config.Service.Name, rt.nodeID)
}

func (rt Runtime) VolumeDir() string {
	return fmt.Sprintf("./logs/%s", rt.Title())
}
