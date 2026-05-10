package broker

import (
	"context"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"

	"connectrpc.com/connect"
)

// Query executes a CESQL expression against stored events and returns all matches.
func (h *rpc) Query(ctx context.Context, req *connect.Request[acv1.QueryRequest]) (*connect.Response[acv1.QueryResponse], error) {
	h.logger.Info("query request")

	events, err := h.controller.Query(ctx, req.Msg)
	if err != nil {
		h.logger.Error(err.Error())
		return nil, err
	}

	return connect.NewResponse(&acv1.QueryResponse{Events: events}), nil
}

// QueryStream replays stored events then streams live events as they arrive.
func (h *rpc) QueryStream(ctx context.Context, req *connect.Request[acv1.QueryRequest], stream *connect.ServerStream[acv1.QueryStreamResponse]) error {
	h.logger.Info("query stream request")
	return h.controller.QueryStream(ctx, req.Msg, stream)
}
