package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	Error error
	Data  interface{}
}

// ==== RPC HANDLER METHODS ==== //

// Set - Responds to the rpc method `Set`. The request is checked to see if it's running on the leader
// if not then an error is returned. After, the leader is validated the payload is transformed to the `CommandPayload`
// and then apply'ed to the raft log. If that is successful then it's considered committed to the cluster.
func (r *keyValueModel) Set(ctx context.Context, req *kvv1.SetRequest) (*kvv1.SetResponse, error) {
	if r.raft.State() != raft.Leader {
		fmt.Println("no leader")
		// todo -> redirect request to leader, or just return an error that the client
		// can then call the leader
		fmt.Println("leader address: ", r.raft.Leader())
		return nil, errors.New("call leader to set data")
	}

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
			return nil
		} else {
			return errors.New("failed to register raft with the service")
		}
	}
	return errors.New("raft connection is nill")
}

func (r *keyValueModel) RegisterRPC(server *grpc.Server) {
	kvv1.RegisterKeyValueServiceServer(server, r)
}

// Implement the the `draft.RepoRegister` interface so that the underlying infrastructure
// is put into place before the service is running.
func (r *keyValueModel) RegisterRepo(dbConn interface{}) error {
	if dbConn != nil {
		// todo -> figure out why I'm getting a !ok here
		if db, ok := dbConn.(*badger.DB); ok {
			r.db = db
			return nil
		} else {
			return errors.New("db connection is not the expected type")
		}
	}
	return errors.New("db connection is nil")
}

// ==== MODEL OPERATIONS ==== //
func (r *keyValueModel) set(key string, value interface{}) error {
	var data = make([]byte, 0)
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if data == nil || len(data) <= 0 {
		return nil
	}

	txn := r.db.NewTransaction(true)
	err = txn.Set([]byte(key), data)
	if err != nil {
		txn.Discard()
		return err
	}

	return txn.Commit()
}

// ==================================================
// Implementation of `raft.FSM` interface
// https://github.com/hashicorp/raft/blob/main/fsm.go
// ==================================================

func (r *keyValueModel) Apply(log *raft.Log) interface{} {
	switch log.Type {
	case raft.LogCommand:
		var payload = CommandPayload{}
		if err := json.Unmarshal(log.Data, &payload); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error marshalling store payload %s\n", err.Error())
			return nil
		}

		switch payload.Operation {
		case Set:
			if err := r.set(payload.Key, payload.Value); err != nil {
				fmt.Println(err)
				return &ApplyResponse{
					Error: err,
					Data:  payload,
				}
			} else {
				return &ApplyResponse{
					Error: nil,
					Data:  payload,
				}
			}

		case Get:
		case NullOperation:
			fmt.Println("null operation received from log")
		}
	}

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
