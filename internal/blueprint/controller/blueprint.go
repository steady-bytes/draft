package controller

import (
	"errors"

	m "github.com/steady-bytes/draft/blueprint/model"

	"github.com/hashicorp/raft"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	Controller interface {
		draft.ConsensusRegistrar
		raft.FSM

		Blueprint
	}

	Blueprint interface {
		KeyValueController
	}

	controller struct {
		db   m.KeyValueModel
		raft *raft.Raft
	}
)

func New(db m.KeyValueModel) Controller {
	return &controller{
		db: db,
	}
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
