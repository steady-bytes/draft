package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/dgraph-io/badger/v2"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
	"google.golang.org/protobuf/proto"
)

// KeyValueRepo - Is the integration with badgerDB an embeddable, persistent, and fast key-val database
//
// REF: https://dgraph.io/docs/badger/get-started/

type (
	// KeyValueRepo - Is a generic interface that is integrated with badgerDB.
	// It offers simplified methods that remaining layers of any application can use.
	// The only requirement is that each value that is stored to the db needs to
	// be a `proto.Message` b/c `proto.Marshal` is being used to encode
	KeyValueRepo[T proto.Message] interface {
		draft.RepoRegistrar
		// Delete removes a key forever
		Delete(key string, kind T) error
		// Retrieve a value by it's key
		Get(key string, kind T) (T, error)
		// Query takes in a key prefix, and returns a map
		// of all values that the key prefix matches
		Query(T) (map[string]T, error)
		// Save a key, value to badger. If a key is the same as an existing
		// key that has already been saved then the new value will overwrite the old.
		Set(key string, value T) error
	}

	model[T proto.Message] struct {
		db *badger.DB
	}
)

// New - Initialize a new `KeyValueRepo` struct. NOTE: This will not bootstrap the underlying
// badger database. That will happen when `RegisterRepo` is called in the service chassis when
// the service starts up.
func New[T proto.Message]() KeyValueRepo[T] {
	return &model[T]{}
}

var (
	ErrInvalidKeyLength     = errors.New("key's can't be a length of 0")
	ErrIncorrectDBInterface = errors.New("incorrect db interface")
	ErrDBNilDBConnection    = errors.New("db connection is nil")
	ErrFailedDelete         = errors.New("failed to delete key value pair")
	ErrFailedGet            = errors.New("failed to get value for key")
)

// Implement the the `draft.RepoRegister` interface so that the underlying infrastructure is put
// into place before the application is run.
func (m *model[T]) RegisterRepo(dbConn interface{}) error {
	if dbConn != nil {
		if db, ok := dbConn.(*badger.DB); ok {
			m.db = db
			return nil
		} else {
			return ErrIncorrectDBInterface
		}
	}
	return ErrDBNilDBConnection
}

// Delete - Takes a key, and a model to locate persistance layer. If found and the delete operation
// is successful an error is not returned. Otherwise, and error will return.
func (m *model[T]) Delete(key string, kind T) error {
	var (
		txn     = m.db.NewTransaction(true)
		keyByte = MakeKey(key, kind)
		err     error
	)
	defer txn.Commit()

	if err = txn.Delete(keyByte); err != nil {
		fmt.Println("error: ", err)
		return ErrFailedDelete
	}

	return nil
}

func (m *model[T]) Get(k string, kind T) (T, error) {
	if len(k) == 0 {
		return kind, ErrInvalidKeyLength
	}

	var (
		keyByte = MakeKey(k, kind)
		txn     = m.db.NewTransaction(false)
		err     error
		key     *badger.Item
		t       = kind
	)
	defer txn.Commit()

	key, err = txn.Get(keyByte)
	if err != nil {
		fmt.Println("error: ", err)
		return kind, ErrFailedGet
	}

	err = key.Value(func(v []byte) error {
		if err := proto.Unmarshal(v, t); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return kind, err
	}

	return t, err
}

func MakeKey[T any](key string, kind T) []byte {
	return []byte(reflect.TypeOf(kind).String() + key)
}

// Query - Takes a key prefix
func (m *model[T]) Query(kind T) (map[string]T, error) {
	var (
		opts   = badger.DefaultIteratorOptions
		txn    = m.db.NewTransaction(true)
		it     = txn.NewIterator(opts)
		output = make(map[string]T)
	)
	defer it.Close()

	prefix := proto.MessageName(kind)

	// This should iterate over keys only if set to false
	// opts.PrefetchValues = true

	for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
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

func (m *model[T]) Set(k string, value T) error {
	var (
		key  = MakeKey(k, value)
		data = make([]byte, 0)
		txn  = m.db.NewTransaction(true)
	)

	data, err := proto.Marshal(value)
	if err != nil {
		return err
	}

	if data == nil || len(data) <= 0 {
		return nil
	}

	err = txn.Set(key, data)
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
