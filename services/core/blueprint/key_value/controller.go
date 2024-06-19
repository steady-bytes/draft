package key_value

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	fsv1 "github.com/steady-bytes/draft/api/core/consensus/fsm/v1"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Cnt "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	Controller interface {
		chassis.ConsensusRegistrar
		raft.FSM

		KeyValue
	}

	KeyValue interface {
		Delete(log chassis.Logger, key string, value T) error
		Set(log chassis.Logger, key string, value T, timeout time.Duration) (*SetResponse, error)
		Get(log chassis.Logger, key string, value T) (T, error)
		List(log chassis.Logger, kind T) (map[string]T, error)
	}

	SetResponse struct {
		Error error
		Data  interface{}
	}

	controller struct {
		model   Model
		raft    *raft.Raft
	}
)

const (
	NullOperation = iota
	Set
	Delete
)

var (
	ErrFailedLSMLogBuild = errors.New("failed to build the raft log from the key/value provided")
	ErrFailedAnyCast     = errors.New("failed to cast the value to anypb")
	ErrFailedToMarshal   = errors.New("failed to marshal payload")
)

func NewController(model Model) Controller {
	return &controller{
		model: model,
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

func (c *controller) LeadershipChange(log chassis.Logger, leader bool, address string) {
	if leader {
		log.Info("became leader")
		value, err := anypb.New(&kvv1.Value{
			Data: address,
		})
		if err != nil {
			log.WithError(err).Error("failed to create any type from value")
			return
		}
		// write the grpc address and port of the grpc service to raft
		_, err = c.Set(log, "leader", value, 500*time.Millisecond)
		if err != nil {
			log.WithError(err).Error("failed to set leader address")
		}
	} else {
		log.Info("become follower")
	}
}

func (c *controller) Delete(log chassis.Logger, key string, kind T) error {
	if err := c.model.Delete(key, kind); err != nil {
		return err
	}
	return nil
}

func (c *controller) Get(log chassis.Logger, key string, value T) (T, error) {
	val, err := c.model.Get(key, value)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	return val, nil
}

func (c *controller) Set(log chassis.Logger, key string, value T, timeout time.Duration) (*SetResponse, error) {
	// forward the set request to the leader if we are not the leader
	if c.raft.State() != raft.Leader {
		log.Info("forwarding set request to leader")
		// create a client to the current leader
		a, _ := anypb.New(&kvv1.Value{})
		anyValue, err := c.model.Get("leader", a)
		if err != nil {
			log.WithError(err).Error("failed to get leader address")
			return nil, err
		}
		v := &kvv1.Value{}
		err = anypb.UnmarshalTo(anyValue, v, proto.UnmarshalOptions{})
		if err != nil {
			log.WithError(err).Error("failed to unmarshal leader value")
			return nil, err
		}
		client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, v.Data)

		// forward the set request to the leader
		req := connect.NewRequest(&kvv1.SetRequest{
			Key:   key,
			Value: value,
		})
		_, err = client.Set(context.Background(), req)
		if err != nil {
			log.WithError(err).Error("failed to forward set request to leader")
			return nil, err
		}

		return nil, nil
	}

	// build lsm log
	lsmLog, err := c.buildLSMLog(key, value, fsv1.Operation_SET)
	if err != nil {
		log.Error(ErrFailedLSMLogBuild.Error())
		return nil, ErrFailedLSMLogBuild
	}

	future := c.raft.Apply(lsmLog, timeout)
	if err := future.Error(); err != nil {
		log.Error(err.Error())
		return nil, errors.New("failed to apply command")
	}

	res, ok := future.Response().(*SetResponse)
	if !ok {
		return nil, errors.New("failed to apply command")
	}

	if res.Error != nil {
		log.Error(res.Error.Error())
		return nil, res.Error
	}

	return res, nil
}

func (c *controller) buildLSMLog(key string, value T, operation fsv1.Operation) ([]byte, error) {
	payload := &fsv1.CommandPayload{
		Operation: operation,
		Key:       key,
		Value:     value,
	}

	data, err := proto.Marshal(payload)
	if err != nil {
		return nil, ErrFailedToMarshal
	}

	return data, nil
}

func (c *controller) List(log chassis.Logger, kind T) (map[string]T, error) {
	keyValMap, err := c.model.List(kind)
	if err != nil {
		log.Error(err.Error())
		return nil, ErrFailedList
	}

	return keyValMap, nil
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
			if err := c.model.Set(payload.Key, payload.Value); err != nil {
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
