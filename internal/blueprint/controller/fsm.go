package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hashicorp/raft"
)

type (
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
