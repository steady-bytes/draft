package key_value

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v2"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// KeyValueRepo - Is the integration with badgerDB an embeddable, persistent, and fast key-val database
//
// REF: https://dgraph.io/docs/badger/get-started/

type (
	// T is a type alias for the `anypb.Any` struct so that additional methods can be added to
	// the message type.
	T = *anypb.Any

	// Key is an alias to a string that identifies it's use as a key in the key/value store
	Key = string

	// A generic interface that is integrated with badgerDB.
	// It offers simplified methods that remaining layers of any application can use.
	// The only requirement is that each value that is stored to the db needs to
	// be a `proto.Message` b/c `proto.Marshal` is being used to encode
	Repo interface {
		draft.RepoRegistrar
		keyValue
	}

	keyValue interface {
		// Delete removes a key forever
		Delete(Key, T) error
		// Retrieve a value by it's key
		Get(Key, T) (T, error)
		// Query takes in a key prefix, and returns a map
		// of all values that the key prefix matches
		Query(T) (map[Key]T, error)
		// Save a key, value to badger. If a key is the same as an existing
		// key that has already been saved then the new value will overwrite the old.
		Set(Key, T) error
	}

	// structure implementing the `KeyValueRepo` interface for the type `T` of `proto.Message`
	repo struct {
		db *badger.DB
	}
)

// New - Initialize a new `KeyValueRepo` struct. NOTE: This will not bootstrap the underlying
// badger database. That will happen when `RegisterRepo` is called in the service chassis when
// the service starts up.
func NewRepo() Repo {
	return &repo{
		db: nil,
	}
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
func (m *repo) RegisterRepo(dbConn interface{}) error {
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

// Delete - Takes a key, and a repo to locate persistance layer. If found and the delete operation
// is successful an error is not returned. Otherwise, and error will return.
func (m *repo) Delete(k Key, kind T) error {
	var (
		txn     = m.db.NewTransaction(true)
		keyByte = m.makeKey(k, kind)
		err     error
	)
	defer txn.Commit()

	if err = txn.Delete(keyByte); err != nil {
		fmt.Println("error: ", err)
		return ErrFailedDelete
	}

	return nil
}

func (m *repo) Get(k string, kind T) (T, error) {
	if len(k) == 0 {
		return kind, ErrInvalidKeyLength
	}

	var (
		keyByte = m.makeKey(k, kind)
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

func (m *repo) makeKey(key string, kind *anypb.Any) []byte {
	key = strings.ReplaceAll(key, " ", "")
	return []byte(kind.GetTypeUrl() + "-" + key)
}

func (m *repo) Query(kind T) (map[string]T, error) {
	var (
		opts   = badger.DefaultIteratorOptions
		txn    = m.db.NewTransaction(true)
		it     = txn.NewIterator(opts)
		output = make(map[string]T)
	)
	defer it.Close()

	prefix := ""

	// This should iterate over keys only if set to false
	// opts.PrefetchValues = true

	for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
		item := it.Item()
		k := item.Key()
		err := item.Value(func(v []byte) error {

			// TODO -> Unmarshal the type either `Struct`, `sdv`.Process

			// Unwrap the any
			// Check to see if type is

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

func (m *repo) Set(k string, value T) error {
	var (
		key  = m.makeKey(k, value)
		txn  = m.db.NewTransaction(true)
		data = make([]byte, 0)
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
