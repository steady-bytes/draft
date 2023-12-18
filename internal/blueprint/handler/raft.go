package handler

import (
	"context"
	"errors"

	cnt "connectrpc.com/connect"
	rfv1 "github.com/steady-bytes/draft/api/gen/go/consensus/raft/v1"
)

func (h *handler) Join(ctx context.Context, req *cnt.Request[rfv1.JoinRequest]) (*cnt.Response[rfv1.JoinResponse], error) {
	return nil, errors.New("implement me")
}

func (h *handler) Remove(ctx context.Context, req *cnt.Request[rfv1.RemoveRequest]) (*cnt.Response[rfv1.RemoveResponse], error) {
	return nil, errors.New("implement me")
}

func (h *handler) Stats(ctx context.Context, req *cnt.Request[rfv1.StatsRequest]) (*cnt.Response[rfv1.StatsResponse], error) {
	return nil, errors.New("implement me")
}
