package model

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v2"
	sdv1 "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

// I hate the name of this, it's not a model.... It's the way

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
		Iterate(prefix []byte)
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

/*
db.View(func(txn *badger.Txn) error {
  it := txn.NewIterator(badger.DefaultIteratorOptions)
  defer it.Close()
  prefix := []byte("1234")
  for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
    item := it.Item()
    k := item.Key()
    err := item.Value(func(v []byte) error {
      fmt.Printf("key=%s, value=%s\n", k, v)
      return nil
    })
    if err != nil {
      return err
    }
  }
  return nil
})
*/

// If I'm going to make this generic then it's going to need to use reflection to lookup the
// key's and it's value and do a comparison on them

func (m *model) Iterate(prefix []byte) error {
	opts := badger.DefaultIteratorOptions
	// This should iterate over keys only if set to false
	// opts.PrefetchValues = true

	txn := m.db.NewTransaction(true)
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		k := item.Key()
		err := item.Value(func(v []byte) error {
			fmt.Printf("key=%s value=%s", k, v)
			// TODO -> Make sure this is the most efficient way to unpack a message that is written to to db
			var process sdv1.Process
			err := json.Unmarshal(v, &process)
			if err != nil {
				fmt.Println(err)
			}

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
