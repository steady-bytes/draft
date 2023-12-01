package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	m "github.com/steady-bytes/draft/blueprint/model"

	"github.com/hashicorp/raft"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	KeyValueController interface {
		draft.ConsensusRegistrar
		raft.FSM

		Set(data []byte, timeout time.Duration) (*ApplyResponse, error)
		Get(key string) ([]byte, error)
	}

	controller struct {
		db   m.KeyValueModel
		raft *raft.Raft
	}

	CommandPayload struct {
		Operation Operation
		Key       string
		Value     interface{}
	}

	Operation int32

	ApplyResponse struct {
		Error error
		Data  interface{}
	}
)

const (
	NullOperation = iota
	Set
	Get
	Delete
)

func New(db m.KeyValueModel) KeyValueController {
	return &controller{
		db: db,
	}
}

// Implement the the `draft.ConsensusRegister` interface so that the underlying infrastructure
// is put into place before the service is running. To run this service as a replicated service
// that can share, and agree on.
func (c *controller) RegisterConsensus(raftConn interface{}) error {
	if raftConn != nil {
		if raft, ok := raftConn.(*raft.Raft); ok {
			c.raft = raft
			return nil
		} else {
			return errors.New("failed to register raft with the service")
		}
	}
	return errors.New("raft connection is nill")
}

func (c *controller) Set(data []byte, timeout time.Duration) (*ApplyResponse, error) {
	if c.raft.State() != raft.Leader {
		fmt.Println("no leader")
		// todo -> redirect request to leader, or just return an error that the client
		// can then call the leader
		fmt.Println("leader address: ", c.raft.Leader())
		return nil, errors.New("call leader to set data")
	}

	future := c.raft.Apply(data, timeout)
	if err := future.Error(); err != nil {
		fmt.Println(err)
		return nil, errors.New("failed to apply command")
	}

	res, ok := future.Response().(*ApplyResponse)
	if !ok {
		return nil, errors.New("failed to apply command")
	}

	return res, nil
}

func (c *controller) Get(key string) ([]byte, error) {
	val, err := c.db.Get(key)
	if err != nil {
		fmt.Println("error: ", err)
		return nil, err
	}

	return val, nil
}

func (c *controller) Apply(log *raft.Log) interface{} {
	switch log.Type {
	case raft.LogCommand:
		var payload = CommandPayload{}
		if err := json.Unmarshal(log.Data, &payload); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error marshalling store payload %s\n", err.Error())
			return nil
		}

		switch payload.Operation {
		case Set:
			if err := c.db.Set(payload.Key, payload.Value); err != nil {
				fmt.Println(err)

				return &ApplyResponse{
					Error: errors.New("failed to set key/val"),
					Data:  payload,
				}
			} else {
				return &ApplyResponse{
					Error: nil,
					Data:  payload,
				}
			}

		case Get:
			fallthrough
		case NullOperation:
			fmt.Println("null operation received from log")
			return nil
		}
	}

	return nil
}

func (c *controller) Snapshot() (raft.FSMSnapshot, error) {
	return newSnapshotNoop()
}

func (c *controller) Restore(rClose io.ReadCloser) error {
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
