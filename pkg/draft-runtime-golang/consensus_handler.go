package draft_runtime_golang

import (
	"context"
	"errors"

	rfv1 "github.com/steady-bytes/draft/api/gen/go/consensus/raft/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type (
	RaftRPCHandler interface {
		RPCRegistrar
		rfv1.RaftServiceServer
	}
	raftRPCHandler struct {
		rfv1.UnimplementedRaftServiceServer

		raftController RaftController
	}
)

func NewRaftRPCHandler(raftController RaftController) RaftRPCHandler {
	return &raftRPCHandler{
		raftController: raftController,
	}
}

func (r *raftRPCHandler) RegisterRPC(server *grpc.Server) {
	rfv1.RegisterRaftServiceServer(server, r)
	reflection.Register(server)
}

func (r *raftRPCHandler) Join(ctx context.Context, req *rfv1.JoinRequest) (*rfv1.JoinResponse, error) {
	if err := r.raftController.Join(ctx, req.GetNodeId(), req.GetRaftAddress()); err != nil {
		return nil, errors.New("failed to join cluster")
	}

	return &rfv1.JoinResponse{
		NodeId:      req.GetNodeId(),
		RaftAddress: req.GetRaftAddress(),
	}, nil
}

func (r *raftRPCHandler) Remove(ctx context.Context, req *rfv1.RemoveRequest) (*rfv1.RemoveResponse, error) {
	return nil, errors.New("implement me")
}

func (r *raftRPCHandler) Stats(ctx context.Context, req *rfv1.StatsRequest) (*rfv1.StatsResponse, error) {
	return nil, errors.New("implement me")
}
