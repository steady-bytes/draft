package chassis

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	rfv1 "github.com/steady-bytes/draft/api/core/consensus/raft/v1"
	rfConnect "github.com/steady-bytes/draft/api/core/consensus/raft/v1/v1connect"
)

type (
	RaftRPCHandler interface {
		RPCRegistrar
		rfConnect.RaftServiceHandler
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

func (r *raftRPCHandler) RegisterRPC(server Rpcer) {
	// NOTE: don't enable reflection for this handler. It's using a separate `tcp` connection then
	// http, and rpc interfaces implemented at the service level
	pattern, handler := rfConnect.NewRaftServiceHandler(r)
	server.AddHandler(pattern, handler, false)
}

func (r *raftRPCHandler) Join(ctx context.Context, req *connect.Request[rfv1.JoinRequest]) (*connect.Response[rfv1.JoinResponse], error) {
	var (
		nodeID      = req.Msg.GetNodeId()
		raftAddress = req.Msg.GetRaftAddress()
	)

	if err := r.raftController.Join(ctx, nodeID, raftAddress); err != nil {
		return nil, errors.New("failed to join cluster")
	}

	return connect.NewResponse(&rfv1.JoinResponse{
		NodeId:      nodeID,
		RaftAddress: raftAddress,
	}), nil
}

func (r *raftRPCHandler) Remove(
	ctx context.Context,
	req *connect.Request[rfv1.RemoveRequest],
) (*connect.Response[rfv1.RemoveResponse], error) {
	return nil, errors.New("implement me")
}

func (r *raftRPCHandler) Stats(
	ctx context.Context,
	req *connect.Request[rfv1.StatsRequest],
) (*connect.Response[rfv1.StatsResponse], error) {
	var (
		nodeID = req.Msg.GetNodeId()
	)

	stats := r.raftController.Stats(ctx)

	return connect.NewResponse(&rfv1.StatsResponse{
		NodeId: nodeID,
		Stats: &rfv1.Stats{
			Stats: stats,
		},
	}), nil
}
