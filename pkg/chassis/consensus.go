package chassis

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/spf13/viper"
)

type ConsensusRegistrar interface {
	RegisterConsensus(interface{}) error
	// LeadershipChange is a callback for the consensus system to notify when a leadership change is made. The
	// leader parameter is true when the node becomes the leader, and false when it becomes a follower. The address
	// parameter is the URL of this node and should be persisted to raft for other nodes to forward write requests to.
	LeadershipChange(log Logger, leader bool, address string)
	raft.FSM
}

type ConsensusKind int

const (
	NullConsensusKind ConsensusKind = iota
	Raft
)

// String - get the human readable value for `ConsensusKind`
func (ck ConsensusKind) String() string {
	return []string{"null", "raft"}[ck]
}

func (c *Runtime) withConsensus(kind ConsensusKind, registrar ConsensusRegistrar) {
	c.consensusKind = kind

	switch c.consensusKind {
	case NullConsensusKind:
		return
	case Raft:
		c.bootstrapRaft(registrar)
	}
}

const (
	// The maxPool controls how many connections we will pool.
	maxPool = 3
	// The timeout is used to apply I/O deadlines. For InstallSnapshot, we multiply
	// the timeout by (SnapshotSize / TimeoutScale).
	// https://github.com/hashicorp/raft/blob/v1.1.2/net_transport.go#L177-L181
	tcpTimeout = 10 * time.Second
	// The `retain` parameter controls how many
	// snapshots are retained. Must be at least 1.
	raftSnapShotRetain = 2
	// raftLogCacheSize is the maximum number of logs to cache in-memory.
	// This is used to reduce disk I/O for the recently committed entries.
	raftLogCacheSize = 512

	invalidRaftAddress = "raft.port and raft.node-id are required but were not provided"
)

func (c *Runtime) bootstrapRaft(registrar ConsensusRegistrar) {
	viper.SetDefault("raft.scheme", "http")
	viper.SetDefault("raft.host", "localhost")
	var (
		raftConf    = raft.DefaultConfig()
		raftScheme  = c.config.GetString("raft.scheme")
		raftPort    = c.config.GetString("raft.port")
		raftHost    = c.config.GetString("raft.host")
		raftNodeID  = c.config.GetString("raft.node-id")
		raftBinAddr = ""
	)

	// listen on the raft notify channel for leadership changes
	notifyCh := make(chan bool)
	raftConf.NotifyCh = notifyCh
	go func(scheme, address, port string) {
		url := fmt.Sprintf("%s://%s:%s", scheme, address, port)
		c.logger.Info("listening for leadership changes")
		for leader := range notifyCh {
			registrar.LeadershipChange(c.logger, leader, url)
		}
	}(raftScheme, raftHost, c.config.GetString("service.port"))

	// configuration for raft
	if raftPort == "" || raftNodeID == "" {
		c.logger.Fatal(invalidRaftAddress)
	} else {
		raftBinAddr = fmt.Sprintf("%s:%s", raftHost, raftPort)
	}

	raftConf.LocalID = raft.ServerID(raftNodeID)
	raftConf.SnapshotThreshold = 1024

	// set the path to the directory bolt will use to write to the filesystem
	basePath := c.config.GetString("badger.path")
	if basePath == "" {
		basePath = raftNodeID
	}
	store, err := raftboltdb.NewBoltStore(filepath.Join(basePath, "raft.dataRepo"))
	if err != nil {
		c.logger.Fatal(err.Error())
	}

	// wrap the store in a `LogCache`` to improve performance
	cacheStore, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		c.logger.Fatal(err.Error())
	}

	// TODO -> understand this more when diving into snapshots, and how they can be used to recover data
	snapshotStore, err := raft.NewFileSnapshotStore(basePath, raftSnapShotRetain, os.Stdout)
	if err != nil {
		c.logger.Fatal(err.Error())
	}

	// create raft address to advertise on
	tcpAddr, err := net.ResolveTCPAddr("tcp", raftBinAddr)
	if err != nil {
		c.logger.Fatal(err.Error())
	}

	c.raftAdvertiseAddress = tcpAddr

	// create the raft tcp transport sub-system, networking configuration
	// of the raft servers
	transport, err := raft.NewTCPTransport(raftBinAddr, c.raftAdvertiseAddress, maxPool, tcpTimeout, os.Stdout)
	if err != nil {
		c.logger.Fatal(err.Error())
	}

	raftServer, err := raft.NewRaft(raftConf, registrar, cacheStore, store, snapshotStore, transport)
	if err != nil {
		c.logger.Fatal(err.Error())
	}
	// always start single server
	// The server will not be added to the cluster until the `Join` rpc method is called.
	// That will trigger the leader election process as well.
	configuration := raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      raft.ServerID(raftNodeID),
				Address: transport.LocalAddr(),
			},
		},
	}

	bootstrap := c.config.GetBool("raft.bootstrap")
	if bootstrap {
		raftServer.BootstrapCluster(configuration)
	}

	// todo -> figure out how to let the upper layer of the service implement `raft.FSM` so that it can determine how
	// the storage facility works
	registrar.RegisterConsensus(raftServer)

	// todo -> The self created rpc raft interface needs to be on a different port then the raft server
	c.RaftController = NewRaftController(raftServer)
	raftHandler := NewRaftRPCHandler(c.RaftController)

	c.withRpc(raftHandler)
}
