package key_value

import (
	"context"
	"fmt"
	"strings"

	"github.com/steady-bytes/draft/pkg/chassis"
	dbadger "github.com/steady-bytes/draft/pkg/repositories/badger"

	"github.com/dgraph-io/badger/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	// T is a type alias for the `anypb.Any` struct so that additional methods can be added to
	// the message type.
	T = *anypb.Any

	// Key is an alias to a string that identifies it's use as a key in the key/value store
	Key = string

	Model interface {
		dbadger.Repository
		// Delete removes a key forever
		Delete(Key, T) error
		// Retrieve a value by it's key
		Get(Key, T) (T, error)
		// List takes in a key prefix, and returns a map
		// of all values that the key prefix matches
		List(T) (map[Key]T, error)
		// Save a key, value to badger. If a key is the same as an existing
		// key that has already been saved then the new value will overwrite the old.
		Set(Key, T) error
	}
	model struct {
		repository dbadger.Repository
	}
)

// New instantiates a new repository. A call to Open is required before use.
func NewModel() Model {
	return &model{
		repository: dbadger.New(),
	}
}

////////////////////////////////////
// Chassis Repository Implementation
////////////////////////////////////

func (m *model) Client() *badger.DB {
	return m.repository.Client()
}

func (m *model) Open(ctx context.Context, config chassis.Config) error {
	return m.repository.Open(ctx, config)
}

func (m *model) Close(ctx context.Context) error {
	return m.repository.Close(ctx)
}

func (m *model) Ping(ctx context.Context) error {
	return m.repository.Ping(ctx)
}

/////////////////////////////////
// Key/Value Model Implementation
/////////////////////////////////

// Delete - Takes a key, and a repo to locate persistance layer. If found and the delete operation
// is successful an error is not returned. Otherwise, and error will return.
func (m *model) Delete(k Key, kind T) error {
	var (
		txn     = m.Client().NewTransaction(true)
		keyByte = m.makeKey(k, kind)
		err     error
	)
	defer txn.Commit()

	if err = txn.Delete(keyByte); err != nil {
		return fmt.Errorf("failed to delete key value pair: %s", err.Error())
	}

	return nil
}

func (m *model) Get(k string, kind T) (T, error) {
	if len(k) == 0 {
		return kind, fmt.Errorf("keys must have length greater than zero")
	}

	var (
		keyByte = m.makeKey(k, kind)
		txn     = m.Client().NewTransaction(false)
		err     error
		key     *badger.Item
		t       = kind
	)
	defer txn.Commit()

	key, err = txn.Get(keyByte)
	if err != nil {
		return t, fmt.Errorf("failed to get value for key: %s", err.Error())
	}

	err = key.Value(func(v []byte) error {
		if err := proto.Unmarshal(v, t); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return t, err
	}

	return t, err
}

func (m *model) makeKey(key string, kind *anypb.Any) []byte {
	key = strings.ReplaceAll(key, " ", "")
	return []byte(kind.GetTypeUrl() + "-" + key)
}

func (m *model) List(kind T) (map[string]T, error) {
	var (
		opts   = badger.DefaultIteratorOptions
		txn    = m.Client().NewTransaction(true)
		it     = txn.NewIterator(opts)
		output = make(map[string]T)
		prefix = kind.GetTypeUrl()
	)
	defer it.Close()

	for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
		item := it.Item()
		k := item.Key()
		err := item.Value(func(v []byte) error {
			var t anypb.Any
			if err := proto.Unmarshal(v, &t); err != nil {
				return err
			}

			output[string(k)] = &t

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return output, nil
}

func (m *model) Set(k string, value T) error {
	var (
		key  = m.makeKey(k, value)
		txn  = m.Client().NewTransaction(true)
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
