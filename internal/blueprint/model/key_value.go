package model

import (
	"encoding/json"
	"errors"
	"fmt"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"github.com/dgraph-io/badger/v2"
)

// KeyValueModel - Is the integration interface with badgerDB that is used to write key/value pairs to the file system
//
// REF: https://dgraph.io/docs/badger/get-started/

type (
	KeyValueModel interface {
		draft.RepoRegistrar
		// Delete removes a key forever
		Delete(key string) error
		// Retrieve a value by it's key
		Get(key string) ([]byte, error)
		// TODO -> resume here, it's worth putting together
		// Iterate
		Iterate()
		// Save a key, value to badger. If a key is the same as an existing
		// key that has already been saved then the new value will overwrite the old.
		Set(key string, value interface{}) error
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

func (m *model) Delete(key string) error {
	var keyByte = []byte(key)

	txn := m.db.NewTransaction(true)
	if err := txn.Delete(keyByte); err != nil {
		fmt.Println(err)
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

func (m *model) Iterate() {
	opts := badger.DefaultIteratorOptions
	// This should iterate over keys only
	opts.PrefetchValues = false

	txn := m.db.NewTransaction(true)
	it := txn.NewIterator(opts)
	defer it.Close()

	// Start at the beginning (i.e. `.Rewind()`) of the LSM tree, and while still valid iterate over
	// each item with `.Next()`
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		k := item.Key()
		err := item.Value(func(v []byte) error {
			fmt.Printf("key=%s", k)
			return nil
		})

		if err != nil {
			fmt.Println(err)
			return
		}
	}
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
