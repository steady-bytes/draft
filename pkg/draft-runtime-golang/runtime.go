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
	"google.golang.org/grpc"
)

// TODO -> add logger with json formatting
// TODO -> add health check on background thread and tie it to readiness, and health checks
// TODO -> implement a graceful shutdown process
// TODO -> add ssl support on postgres

type Runtime struct {
	config *Config
	nodeID string

	tcp net.Listener

	badger   *badger.DB
	gorm     *gorm.DB
	bun      *bun.DB
	repoKind RepoKind

	rpc       *grpc.Server
	rpcServer *http.Server

	nats *nats.Conn
	http *gin.Engine

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
		rpc:    nil,
		tcp:    nil,
		http:   nil,
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
