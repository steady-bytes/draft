package draft_runtime_golang

import (
	"fmt"
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
)

func (c *Runtime) bootstrapRaft(registrar ConsensusRegistrar) {
	// current implementation of consensus uses hashicorp raft which means
	// we also require an instance of badger for the WAL cache
	if c.badger == nil {
		return
	}

	// configuration for raft
	// TODO -> figure out how to dynamically set
	var raftBinAddr = fmt.Sprintf(":%d", 12000)

	raftConf := raft.DefaultConfig()
	raftConf.LocalID = raft.ServerID(c.Title())
	raftConf.SnapshotThreshold = 1024

	store, err := raftboltdb.NewBoltStore(filepath.Join("./", "raft.dataRepo"))
	if err != nil {
		panic(err)
	}

	// Wrap the store in a LogCache to improve performance.
	cacheStore, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		panic(err)
	}

	snapshotStore, err := raft.NewFileSnapshotStore(
		c.VolumeDir(),
		raftSnapShotRetain,
		os.Stdout,
	)
	if err != nil {
		panic(err)
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", raftBinAddr)
	if err != nil {
		panic(err)
	}

	transport, err := raft.NewTCPTransport(
		raftBinAddr,
		tcpAddr,
		maxPool,
		tcpTimeout,
		os.Stdout,
	)
	if err != nil {
		panic(err)
	}

	// todo -> figure out how to let the upper layer of the service implement `raft.FSM` so that it can determine how
	// the storage facility works
	registrar.RegisterConsensus(c.badger)

	raftServer, err := raft.NewRaft(
		raftConf,
		// todo -> figure out what is going to implement the FSM interface. I think it's gonna be the controller
		registrar,
		cacheStore,
		store,
		snapshotStore,
		transport,
	)
	if err != nil {
		panic(err)
	}

	// always start single server as a leader
	configuration := raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      raft.ServerID(c.NodeID()),
				Address: transport.LocalAddr(),
			},
		},
	}

	raftServer.BootstrapCluster(configuration)
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
	// 	panic(err)
	// }

	return
}
