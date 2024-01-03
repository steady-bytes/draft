package controller

import (
	"errors"

	"github.com/hashicorp/raft"
	cmap "github.com/orcaman/concurrent-map/v2"
	sdv1 "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1"
	m "github.com/steady-bytes/draft/blueprint/model"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	Controller interface {
		draft.ConsensusRegistrar
		draft.SecretStoreSetter
		raft.FSM

		Blueprint
	}

	Blueprint interface {
		KeyValueController
		ServiceDiscovery
	}

	controller struct {
		db            m.KeyValueModel
		raft          *raft.Raft
		nonce         string
		secretStore   draft.SecretStore
		systemJournal cmap.ConcurrentMap[string, *sdv1.Process]
	}
)

func New(db m.KeyValueModel) Controller {
	return &controller{
		db:            db,
		systemJournal: cmap.New[*sdv1.Process](),
	}
}

// Accepts a `SecretStore` interface and adds it to the controller
func (c *controller) SetSecretStore(s draft.SecretStore) {
	c.secretStore = s
}

// Implement the the `draft.ConsensusRegister` interface so that the underlying infrastructure
// is put into place before the service is running. To run this service as a replicated service
// that can share, and agree on.
func (c *controller) RegisterConsensus(raftConn interface{}) error {
	if raftConn != nil {
		if raft, ok := raftConn.(*raft.Raft); ok {
			c.raft = raft
			return nil
		} else {
			return errors.New("failed to register raft with the service")
		}
	}
	return errors.New("raft connection is nill")
}
