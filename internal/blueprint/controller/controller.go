package controller

import (
	"errors"

	"github.com/hashicorp/raft"
	r "github.com/steady-bytes/draft/blueprint/repo"
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
		KeyValueController[any]
		ServiceDiscovery
	}

	controller struct {
		repo r.KeyValueRepo[any]
		raft *raft.Raft
		sstr draft.SecretStore
	}
)

func New(repo r.KeyValueRepo[any]) Controller {
	return &controller{
		repo: repo,
	}
}

// Accepts a `SecretStore` interface and adds it to the controller
func (c *controller) SetSecretStore(s draft.SecretStore) {
	c.sstr = s
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
