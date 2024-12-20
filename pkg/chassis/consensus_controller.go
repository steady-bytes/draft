package chassis

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/raft"
)

type (
	RaftController interface {
		Join(ctx context.Context, nodeID, raftAddress string) error
		Stats(ctx context.Context) map[string]string

		GetClusterDetails() raft.Configuration
	}

	raftController struct {
		raft *raft.Raft
	}
)

func NewRaftController(r *raft.Raft) RaftController {
	return &raftController{
		raft: r,
	}
}

func (c *raftController) Join(ctx context.Context, nodeID, raftAddress string) error {
	if c.raft.State() != raft.Leader {
		fmt.Println("must join leader")
		return errors.New("must join leader")
	}

	config := c.raft.GetConfiguration()
	if err := config.Error(); err != nil {
		fmt.Println("failed to get configuration")
		return errors.New("failed to get configuration")
	}

	index := c.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddress), 0, 0)
	if index.Error() != nil {
		fmt.Println("failed to add new voter", index.Error())
		return errors.New("failed to add new voter")
	}

	return nil
}

func (c *raftController) Stats(ctx context.Context) map[string]string {
	return c.raft.Stats()
}

func (c *raftController) GetClusterDetails() raft.Configuration {
	return c.raft.GetConfiguration().Configuration()
}
