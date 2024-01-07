package repo

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/dgraph-io/badger/v2"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

// KeyValueRepo - Is the integration with badgerDB an embeddable, persistent, and fast key-val database
//
// REF: https://dgraph.io/docs/badger/get-started/

type (
	// KeyValueRepo - Is a generic interface atop badgerDB. It offers simplified
	// methods that remaining layers of any application can use. Key's have prepend
	// struct name to the key to build specific collections for specific types
	KeyValueRepo[T any] interface {
		draft.RepoRegistrar
		// Delete removes a key forever
		Delete(key string) error
		// Retrieve a value by it's key
		Get(key string) (*T, error)
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

// New - Initialize a new `KeyValueRepo` struct. NOTE: This will not bootstrap the underlying
// badger database. That will happen when `RegisterRepo` is called in the service chassis when
// the service starts up.
func New[T any]() KeyValueRepo[T] {
	return &model[T]{}
}

// Implement the the `draft.RepoRegister` interface so that the underlying infrastructure is put
// into place before the application is run.
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

// Delete - Takes a key, and a model to locate persistance layer. If found and the delete operation
// is successful an error is not returned. Otherwise, and error will return.
func (m *model[T]) Delete(key string, kind T) error {
	var (
		keyByte = MakeKey(key, kind)
		txn     = m.db.NewTransaction(true)
	)

	if err := txn.Delete(keyByte); err != nil {
		return err
	}

	return nil
}

func (m *model[T]) Get(k string, kind T) (*T, error) {
	var (
		keyByte = MakeKey(k, kind)
		txn     = m.db.NewTransaction(false)
		t       T
		err     error
		key     *badger.Item
	)

	defer txn.Commit()

	key, err = txn.Get(keyByte)
	if err != nil {
		return nil, err
	}

	err = key.Value(func(v []byte) error {
		if err := json.Unmarshal(v, t); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &t, err
}

func MakeKey[T any](key string, kind T) []byte {
	return []byte(reflect.TypeOf(kind).String() + key)
}

// Query - Takes a key prefix
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
