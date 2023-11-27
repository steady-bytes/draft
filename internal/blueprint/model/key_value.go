package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc"

	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_val/v1"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	KeyValueModel interface {
		kvv1.KeyValueServiceServer

		draft.ConsensusRegistrar
		draft.RepoRegistrar
		draft.RPCRegistrar

		raft.FSM
	}

	keyValueModel struct {
		kvv1.UnimplementedKeyValueServiceServer

		db   *badger.DB
		raft *raft.Raft
	}
)

func NewKeyValueModel() KeyValueModel {
	return &keyValueModel{}
}

func (r *keyValueModel) Delete(ctx context.Context, req *kvv1.DeleteRequest) (*kvv1.DeleteResponse, error) {
	return nil, errors.New("implement me")
}

func (r *keyValueModel) Get(ctx context.Context, req *kvv1.GetRequest) (*kvv1.GetResponse, error) {
	return nil, errors.New("implement me")
}

type CommandPayload struct {
	Operation Operation
	Key       string
	Value     interface{}
}

type Operation int32

const (
	NullOperation = iota
	Set
	Get
	Delete
)

type ApplyResponse struct {
}

func (r *keyValueModel) Set(ctx context.Context, req *kvv1.SetRequest) (*kvv1.SetResponse, error) {

	// create `fsm.CommandPayload`
	payload := &CommandPayload{
		Operation: Set,
		Key:       req.GetKey(),
		Value:     req.GetValue(),
	}

	// marshal payload
	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	applyFuture := r.raft.Apply(data, 500*time.Millisecond)
	if err := applyFuture.Error(); err != nil {
		fmt.Println(err)
		return nil, errors.New("failed to apply command")
	}

	_, ok := applyFuture.Response().(*ApplyResponse)
	if !ok {
		fmt.Println(err)
		return nil, errors.New("failed to apply command")
	}

	return &kvv1.SetResponse{
		Key: req.GetKey(),
	}, nil
}

// Implement the the `draft.ConsensusRegister` interface so that the underlying infrastructure
// is put into place before the service is running. To run this service as a replicated service
// that can share, and agree on.
func (r *keyValueModel) RegisterConsensus(raftConn interface{}) error {
	if raftConn != nil {
		// this is not working for some reason
		if raft, ok := raftConn.(*raft.Raft); ok {
			r.raft = raft
		} else {
			return errors.New("failed to register raft with the service")
		}
	}

	return nil
}

func (r *keyValueModel) RegisterRPC(server *grpc.Server) {
	kvv1.RegisterKeyValueServiceServer(server, r)
}

// Implement the the `draft.RepoRegister` interface so that the underlying infrastructure
// is put into place before the service is running.
func (r *keyValueModel) RegisterRepo(dbConn interface{}) error {
	if dbConn == nil {
		return errors.New("db interface is nil")
	} else {
		if db, ok := dbConn.(*badger.DB); ok {
			r.db = db
		}
	}

	return nil
}

// ==================================================
// Implementation of `raft.FSM` interface
// https://github.com/hashicorp/raft/blob/main/fsm.go
// ==================================================

func (r *keyValueModel) Apply(log *raft.Log) interface{} {
	// switch/route on command type
	// SET/UPDATE/DELETE commands
	return nil
}

func (r *keyValueModel) Snapshot() (raft.FSMSnapshot, error) {
	return newSnapshotNoop()
}

func (r *keyValueModel) Restore(rClose io.ReadCloser) error {
	return nil
}

// snapshotNoop handle noop snapshot
type snapshotNoop struct{}

// Persist persist to disk. Return nil on success, otherwise return error.
func (s snapshotNoop) Persist(_ raft.SnapshotSink) error { return nil }

// Release release the lock after persist snapshot.
// Release is invoked when we are finished with the snapshot.
func (s snapshotNoop) Release() {}

// newSnapshotNoop is returned by an FSM in response to a snapshotNoop
// It must be safe to invoke FSMSnapshot methods with concurrent
// calls to Apply.
func newSnapshotNoop() (raft.FSMSnapshot, error) {
	return &snapshotNoop{}, nil
}
