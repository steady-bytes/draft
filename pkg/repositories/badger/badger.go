package badger

import (
	"context"
	"fmt"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/dgraph-io/badger/v2"
)

type (
	Repository interface {
		chassis.Repository
		Client() *badger.DB
	}
	repository struct {
		client *badger.DB
	}
)

// New instantiates a new repository. A call to Open is required before use.
func New() Repository {
	return &repository{}
}

func (r *repository) Client() *badger.DB {
	return r.client
}

func (r *repository) Open(ctx context.Context, config chassis.Config) error {
	badgerOpt := badger.DefaultOptions(config.NodeID())
	db, err := badger.Open(badgerOpt)
	if err != nil {
		return err
	}
	r.client = db
	return nil
}

func (r *repository) Close(ctx context.Context) error {
	err := r.client.Close()
	if err != nil {
		return fmt.Errorf("failed to close the db connection for disconnect")
	}
	return nil
}

func (r *repository) Ping(ctx context.Context) error {
	closed := r.client.IsClosed()
	if closed {
		return fmt.Errorf("badger connection is closed")
	}
	return nil
}
