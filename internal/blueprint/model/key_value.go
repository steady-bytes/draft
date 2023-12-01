package model

import (
	"context"
	"encoding/json"
	"errors"

	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_val/v1"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"github.com/dgraph-io/badger/v2"
)

type (
	KeyValueModel interface {
		draft.RepoRegistrar
		// Save a key, value to badger. If a key is the same as an existing
		// key that has already been saved then the new value will overwrite the old.
		Set(key string, value interface{}) error
		// Retrieve a value by it's key
		Get(key string) ([]byte, error)
	}

	model struct {
		db *badger.DB
	}
)

// New - Initialize a new `KeyValueModel` struct
func New() KeyValueModel {
	return &model{}
}

// Implement the the `draft.RepoRegister` interface so that the underlying infrastructure is put into place before the service starts running.
func (m *model) RegisterRepo(dbConn interface{}) error {
	if dbConn != nil {
		if db, ok := dbConn.(*badger.DB); ok {
			m.db = db
			return nil
		} else {
			return errors.New("db connection is not the expected type")
		}
	}
	return errors.New("db connection is nil")
}

func (m *model) Set(key string, value interface{}) error {
	var data = make([]byte, 0)
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if data == nil || len(data) <= 0 {
		return nil
	}

	txn := m.db.NewTransaction(true)
	err = txn.Set([]byte(key), data)
	if err != nil {
		txn.Discard()
		return err
	}

	if err := txn.Commit(); err != nil {
		txn.Discard()
		return err
	}

	return nil
}

func (m *model) Get(key string) ([]byte, error) {
	var keyByte = []byte(key)

	txn := m.db.NewTransaction(false)
	defer func() {
		_ = txn.Commit()
	}()

	item, err := txn.Get(keyByte)
	if err != nil {
		return nil, err
	}

	var value = make([]byte, 0)
	err = item.Value(func(val []byte) error {
		value = append(value, val...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return value, err
}

func (m *model) Delete(ctx context.Context, req *kvv1.DeleteRequest) (*kvv1.DeleteResponse, error) {
	return nil, errors.New("implement me")
}

// type CommandPayload struct {
// 	Operation Operation
// 	Key       string
// 	Value     interface{}
// }

// type Operation int32

// const (
// 	NullOperation = iota
// 	Set
// 	Get
// 	Delete
// )

// type ApplyResponse struct {
// 	Error error
// 	Data  interface{}
// }

// ==== RPC HANDLER METHODS ==== //

// Set - Responds to the rpc method `Set`. The request is checked to see if it's running on the leader
// if not then an error is returned. After, the leader is validated the payload is transformed to the `CommandPayload`
// and then apply'ed to the raft log. If that is successful then it's considered committed to the cluster.
/*
func (r *keyValueModel) Set(ctx context.Context, req *kvv1.SetRequest) (*kvv1.SetResponse, error) {
	var (
		key = strings.TrimSpace(req.GetKey())
	)

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
		Key:       key,
		Value:     req.GetValue(),
	}

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
		Key: key,
	}, nil
}
*/

// Get - Looks for a key that maybe in the `Log` and if found returns the associated value
// func (r *keyValueModel) Get(ctx context.Context, req *kvv1.GetRequest) (*kvv1.GetResponse, error) {
// 	var (
// 		key = strings.TrimSpace(req.GetKey())
// 	)

// 	value, err := r.get(key)
// 	if err != nil {
// 		fmt.Println("error reading: ", err)
// 		return nil, errors.New("failed to get value for key")
// 	}

// 	fmt.Println("value: ", string(value))

// 	return &kvv1.GetResponse{
// 		Response: &kvv1.GetResponse_Data{
// 			Data: &kvv1.Data{
// 				Data: value,
// 			},
// 		},
// 	}, nil
// }

// Implement the the `draft.ConsensusRegister` interface so that the underlying infrastructure
// is put into place before the service is running. To run this service as a replicated service
// that can share, and agree on.
// func (r *keyValueModel) RegisterConsensus(raftConn interface{}) error {
// 	if raftConn != nil {
// 		if raft, ok := raftConn.(*raft.Raft); ok {
// 			r.raft = raft
// 			return nil
// 		} else {
// 			return errors.New("failed to register raft with the service")
// 		}
// 	}
// 	return errors.New("raft connection is nill")
// }

// func (r *keyValueModel) RegisterRPC(server *grpc.Server) {
// 	kvv1.RegisterKeyValueServiceServer(server, r)
// }

// ==== MODEL OPERATIONS ==== //

// ==================================================
// Implementation of `raft.FSM` interface
// https://github.com/hashicorp/raft/blob/main/fsm.go
// ==================================================

// func (r *keyValueModel) Apply(log *raft.Log) interface{} {
// 	switch log.Type {
// 	case raft.LogCommand:
// 		var payload = CommandPayload{}
// 		if err := json.Unmarshal(log.Data, &payload); err != nil {
// 			_, _ = fmt.Fprintf(os.Stderr, "error marshalling store payload %s\n", err.Error())
// 			return nil
// 		}

// 		switch payload.Operation {
// 		case Set:
// 			if err := r.set(payload.Key, payload.Value); err != nil {
// 				fmt.Println(err)

// 				return &ApplyResponse{
// 					Error: errors.New("failed to set key/val"),
// 					Data:  payload,
// 				}
// 			} else {
// 				return &ApplyResponse{
// 					Error: nil,
// 					Data:  payload,
// 				}
// 			}

// 		case Get:
// 			fallthrough
// 		case NullOperation:
// 			fmt.Println("null operation received from log")
// 			return nil
// 		}
// 	}

// 	return nil
// }

// func (r *keyValueModel) Snapshot() (raft.FSMSnapshot, error) {
// 	return newSnapshotNoop()
// }

// func (r *keyValueModel) Restore(rClose io.ReadCloser) error {
// 	return nil
// }

// // snapshotNoop handle noop snapshot
// type snapshotNoop struct{}

// // Persist persist to disk. Return nil on success, otherwise return error.
// func (s snapshotNoop) Persist(_ raft.SnapshotSink) error { return nil }

// // Release release the lock after persist snapshot.
// // Release is invoked when we are finished with the snapshot.
// func (s snapshotNoop) Release() {}

// // newSnapshotNoop is returned by an FSM in response to a snapshotNoop
// // It must be safe to invoke FSMSnapshot methods with concurrent
// // calls to Apply.
// func newSnapshotNoop() (raft.FSMSnapshot, error) {
// 	return &snapshotNoop{}, nil
// }
