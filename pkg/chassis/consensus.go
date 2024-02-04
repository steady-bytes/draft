package chassis

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

type ConsensusRegistrar interface {
	RegisterConsensus(interface{}) error
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
	// raftPortEnv is the env var that is used to configure what port raft will be running on
	raftPortEnv = "RAFT_PORT"
	raftIPEnv   = "RAFT_IP"
	nodeIDEnv   = "RAFT_NODE_ID"

	invalidRaftAddress = "either IP, PORT, NodeID are invalid"
)

func (c *Runtime) bootstrapRaft(registrar ConsensusRegistrar) {
	var (
		raftConf    = raft.DefaultConfig()
		raftPortStr = os.Getenv(raftPortEnv)
		raftIPStr   = os.Getenv(raftIPEnv)
		raftNodeID  = os.Getenv(nodeIDEnv)
		raftBinAddr = ""
	)
	// current implementation of consensus uses hashicorp raft which means
	// we also require an instance of badger. This also right now should be
	// created in the service because the service might want access to what
	// badger is doing on the file system
	if c.badger == nil {
		return
	}

	// configuration for raft
	if raftPortStr == "" || raftIPStr == "" || raftNodeID == "" {
		log.Fatal(errors.New(invalidRaftAddress))
	} else {
		raftBinAddr = fmt.Sprintf("%s:%s", raftIPStr, raftPortStr)
	}

	raftConf.LocalID = raft.ServerID(raftNodeID)
	raftConf.SnapshotThreshold = 1024

	// set the path to the directory bolt will use to write to the filesystem
	store, err := raftboltdb.NewBoltStore(filepath.Join(raftNodeID, "raft.dataRepo"))
	if err != nil {
		log.Fatal(err)
	}

	// wrap the store in a `LogCache`` to improve performance
	cacheStore, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		log.Fatal(err)
	}

	// TODO -> understand this more when diving into snapshots, and how they can be used to recover data
	snapshotStore, err := raft.NewFileSnapshotStore(raftNodeID, raftSnapShotRetain, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	// create raft address to advertise on
	tcpAddr, err := net.ResolveTCPAddr("tcp", raftBinAddr)
	if err != nil {
		log.Fatal(err)
	}

	c.raftAdvertiseAddress = tcpAddr

	// create the raft tcp transport sub-system, networking configuration
	// of the raft servers
	transport, err := raft.NewTCPTransport(raftBinAddr, c.raftAdvertiseAddress, maxPool, tcpTimeout, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	raftServer, err := raft.NewRaft(raftConf, registrar, cacheStore, store, snapshotStore, transport)
	if err != nil {
		log.Fatal(err)
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

	bootstrap := os.Getenv("BOOTSTRAP_RAFT")
	if bootstrap == "true" {
		raftServer.BootstrapCluster(configuration)
	}

	// todo -> figure out how to let the upper layer of the service implement `raft.FSM` so that it can determine how
	// the storage facility works
	registrar.RegisterConsensus(raftServer)

	// todo -> The self created rpc raft interface needs to be on a different port then the raft server
	raftController := NewRaftController(raftServer)
	raftHandler := NewRaftRPCHandler(raftController)

	c.withRpc(raftHandler)

	// the server will be implemented as an rpc interface instead of a rest interface

	// server is it's own custom implementation
	// TODO -> Implement the rpc methods for the raft server
	// 		   This will be local to the runtime given consistency will be
	//         a configuration of the runtime, and not a feature of the server that is
	//         being implemented.

	// srv := server.New(fmt.Sprintf(":%d", c.config.Service.Port), c.badger, raftServer)
	// if err := srv.Start(); err != nil {
	// 	log.Fatal(err)
	// }
}
