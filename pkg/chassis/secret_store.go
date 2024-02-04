package chassis

import (
	"errors"
	"sync"
)

type (
	// This is the interface that will be added to the controller
	SecretStore interface {
		Get(key string) (string, error)
	}
	// TODO -> Refactor this using a conncurent map. It's currently not the best implementation
	// of this kind of store b/c it might cause a deadlock of to many treads are tryiing to
	// access the store. Either making my own tread safe map, or using something already premade like `sync.Map`, or
	// `concurrent-map` will add some protential performance gains.
	//
	// (https://github.com/orcaman/concurrent-map/tree/master)
	secretStore struct {
		mu    sync.Mutex
		store map[string]string
	}

	SecretStoreSetter interface {
		SetSecretStore(SecretStore)
	}
)

const (
	GlobalNonceKey = "GLOBAL_NONCE"
)

func (c *Runtime) withSecretStore(s SecretStoreSetter) {
	s.SetSecretStore(NewSecretStore())
}

func NewSecretStore() SecretStore {
	ss := &secretStore{
		store: make(map[string]string),
	}
	// TODO -> populate what should be in the store
	// TODO -> remove this it's just a temporary hardcoding
	ss.store[GlobalNonceKey] = "BLUEPRINT"

	return ss
}

func (s *secretStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.store[key]
	if !ok {
		return "", errors.New("value for key not found")
	}

	return val, nil
}
