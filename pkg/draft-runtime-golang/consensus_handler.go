package draft_runtime_golang

import (
	"context"
	"errors"
	"net/http"

	cnt "connectrpc.com/connect"

	rfv1 "github.com/steady-bytes/draft/api/gen/go/consensus/raft/v1"
	connect "github.com/steady-bytes/draft/api/gen/go/consensus/raft/v1/v1connect"
)

type (
	RaftRPCHandler interface {
		RPCRegistrar
		connect.RaftServiceHandler
	}
	raftRPCHandler struct {
		raftController RaftController
	}
)

func NewRaftRPCHandler(raftController RaftController) RaftRPCHandler {
	return &raftRPCHandler{
		raftController: raftController,
	}
}

func (r *raftRPCHandler) RegisterRPC(server *http.ServeMux) {
	// rfv1.RegisterRaftServiceServer(server, r)
	// reflection.Register(server)
	server.Handle(connect.NewRaftServiceHandler(r))
}

func (r *raftRPCHandler) Join(ctx context.Context, req *cnt.Request[rfv1.JoinRequest]) (*cnt.Response[rfv1.JoinResponse], error) {
	var (
		nodeID      = req.Msg.GetNodeId()
		raftAddress = req.Msg.GetRaftAddress()
	)

	if err := r.raftController.Join(ctx, nodeID, raftAddress); err != nil {
		return nil, errors.New("failed to join cluster")
	}

	return cnt.NewResponse(&rfv1.JoinResponse{
		NodeId:      nodeID,
		RaftAddress: raftAddress,
	}), nil
}

func (r *raftRPCHandler) Remove(ctx context.Context, req *cnt.Request[rfv1.RemoveRequest]) (*cnt.Response[rfv1.RemoveResponse], error) {
	return nil, errors.New("implement me")
}

func (r *raftRPCHandler) Stats(ctx context.Context, req *cnt.Request[rfv1.StatsRequest]) (*cnt.Response[rfv1.StatsResponse], error) {
	var (
		nodeID = req.Msg.GetNodeId()
	)

	stats := r.raftController.Stats(ctx)

	return cnt.NewResponse(&rfv1.StatsResponse{
		NodeId: nodeID,
		Stats: &rfv1.Stats{
			Stats: stats,
		},
	}), nil
}
