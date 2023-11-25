package draft_runtime_golang

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/raft"
)

type (
	RaftController interface {
		Join(ctx context.Context, nodeID, raftAddress string) error
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

func (r *raftController) Join(ctx context.Context, nodeID, raftAddress string) error {
	if r.raft.State() != raft.Leader {
		fmt.Println("must join leader")
		return errors.New("must join leader")
	}

	config := r.raft.GetConfiguration()
	if err := config.Error(); err != nil {
		fmt.Println("failed to get configuration")
		return errors.New("failed to get configuration")
	}

	index := r.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddress), 0, 0)
	if index.Error() != nil {
		fmt.Println("failed to add new voter")
		return errors.New("failed to add new voter")
	}

	return nil
}
