package repo

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v2"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

// I hate the name of this, it's not a model.... It's the way

// KeyValueModel - Is the integration interface with badgerDB that is used to write key/value pairs to the file system
//
// REF: https://dgraph.io/docs/badger/get-started/

type (
	KeyValueRepo[T any] interface {
		draft.RepoRegistrar
		// Delete removes a key forever
		Delete(key string) error
		// Retrieve a value by it's key
		Get(key string) ([]byte, error)
		// Query takes in a key prefix, and returns a map
		// of all values that the key prefix matches
		Query(prefix []byte) (map[string]T, error)
		// Save a key, value to badger. If a key is the same as an existing
		// key that has already been saved then the new value will overwrite the old.
		Set(key string, value interface{}) error
	}

	model[T any] struct {
		db *badger.DB
	}
)

// New - Initialize a new `KeyValueRepo` struct
func New[T any]() KeyValueRepo[T] {
	return &model[T]{}
}

// Implement the the `draft.RepoRegister` interface so that the underlying infrastructure is put into place before the service starts running.
func (m *model[T]) RegisterRepo(dbConn interface{}) error {
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

func (m *model[T]) Delete(key string) error {
	var keyByte = []byte(key)

	txn := m.db.NewTransaction(true)
	if err := txn.Delete(keyByte); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func (m *model[T]) Get(key string) ([]byte, error) {
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

func (m *model[T]) Query(prefix []byte) (map[string]T, error) {
	var (
		opts   = badger.DefaultIteratorOptions
		txn    = m.db.NewTransaction(true)
		it     = txn.NewIterator(opts)
		output = make(map[string]T)
	)
	defer it.Close()

	// This should iterate over keys only if set to false
	// opts.PrefetchValues = true

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		k := item.Key()
		err := item.Value(func(v []byte) error {
			var t T
			if err := json.Unmarshal(v, &t); err != nil {
				return err
			}

			output[string(k)] = t

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return output, nil
}

func (m *model[T]) Set(key string, value interface{}) error {
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
