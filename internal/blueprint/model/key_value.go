package model

import (
	"errors"
	"io"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	KeyValueModel interface {
		draft.RepoRegistrar
		raft.FSM
	}

	keyValueModel struct {
		db *badger.DB
	}
)

func NewKeyValueModel() KeyValueModel {
	return &keyValueModel{}
}

func (r *keyValueModel) Set(key, value string) (string, error) {
	return "", errors.New("implement me")
}

func (r *keyValueModel) Get(key, value string) (string, error) {
	return "", errors.New("implement me")
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

// ===========================
// Implementation of `raft.FSM`
// ===========================

func (r *keyValueModel) Apply(log *raft.Log) interface{} {
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
