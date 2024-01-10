package controller

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/hashicorp/raft"
	fsv1 "github.com/steady-bytes/draft/api/gen/go/consensus/fsm/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	KeyValueController interface {
		Delete(key string, value proto.Message) error
		Set(key string, value *anypb.Any, timeout time.Duration) (*SetResponse, error)
		Get(key string) (*proto.Message, error)
		Iterate()
	}

	SetResponse struct {
		Error error
		Data  interface{}
	}
)

const (
	NullOperation = iota
	Set
	Delete
)

var (
	ErrFaildLogBuild = errors.New("failed to build the raft log from the key and value provided")
	ErrFailedAnyCast = errors.New("failed to cast the value to anypb")
)

func (c *controller) Delete(key string, kind proto.Message) error {
	if err := c.repo.Delete(key, kind); err != nil {
		return err
	}

	return nil
}

func (c *controller) Get(key string) (*proto.Message, error) {
	val, err := c.repo.Get(key, &anypb.Any{})
	if err != nil {
		fmt.Println("error: ", err)
		return nil, err
	}

	return &val, nil
}

func (c *controller) Iterate() {
	c.repo.Query(&anypb.Any{})
}

func (c *controller) Set(key string, value *anypb.Any, timeout time.Duration) (*SetResponse, error) {
	if c.raft.State() != raft.Leader {
		fmt.Println("no leader")
		// todo -> redirect request to leader, or just return an error that the client
		// can then call the leader
		fmt.Println("leader address: ", c.raft.Leader())
		return nil, errors.New("call leader to set data")
	}

	// build log
	log, err := c.buildRaftLog(key, value, fsv1.Operation_Set)
	if err != nil {
		return nil, ErrFaildLogBuild
	}

	future := c.raft.Apply(log, timeout)
	if err := future.Error(); err != nil {
		fmt.Println(err)
		return nil, errors.New("failed to apply command")
	}

	res, ok := future.Response().(*SetResponse)
	if !ok {
		return nil, errors.New("failed to apply command")
	}

	if res.Error != nil {
		fmt.Println(res.Error)
		return nil, res.Error
	}

	return res, nil
}

func (c *controller) buildRaftLog(key string, value *anypb.Any, operation fsv1.Operation) ([]byte, error) {
	payload := &fsv1.CommandPayload{
		Operation: operation,
		Key:       key,
		Value:     value,
	}

	data, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.New(ErrfailedToMarshalPayload)
	}

	return data, nil
}

///////////////
// == FSM == //
///////////////

// Implement the `FSM` interface for the `key/value` store so that a change to the leader node will
// be written to each other node joined to the cluster.

// Apply is called when the leader of the cluster received a command that needs to be sent to all of
// the followers in the cluster. Currently in our case the `Set`, and `Delete` invocations of the
// `key/value` store will consume `Apply`
func (c *controller) Apply(log *raft.Log) interface{} {
	switch log.Type {
	case raft.LogCommand:
		var (
			payload = fsv1.CommandPayload{}
			err     error
		)
		if err = proto.Unmarshal(log.Data, &payload); err != nil {
			return &SetResponse{
				Error: errors.New("error marshalling value payload"),
				Data:  payload,
			}
		}

		switch payload.Operation {
		case Delete:
			fmt.Println("TODO: make sure to call the `Apply` command with the `Delete` operations so it's committed to all nodes")
		case Set:
			if err := c.repo.Set(payload.Key, payload.Value); err != nil {
				return &SetResponse{
					Error: errors.New("failed to set key/val"),
					Data:  payload,
				}
			} else {
				return &SetResponse{
					Error: nil,
					Data:  payload,
				}
			}
		case NullOperation:
			fmt.Println("null operation received from log")
			return nil
		}
	}

	return nil
}

// TODO -> figure out how to implement this
func (c *controller) Snapshot() (raft.FSMSnapshot, error) {
	return newSnapshotNoop()
}

// TODO -> figure out how to implement this
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
