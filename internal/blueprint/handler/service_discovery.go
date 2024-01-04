package handler

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	cnt "connectrpc.com/connect"
	sdv1 "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1"
)

func (h *handler) Synchronize(ctx context.Context, stream *cnt.ClientStream[sdv1.ClientDetails]) (*cnt.Response[sdv1.Empty], error) {
	for stream.Receive() {
		h.controller.Synchronize(ctx, stream.Msg())
	}

	// TODO -> handle errors
	if err := stream.Err(); err != nil {
		return nil, cnt.NewError(cnt.CodeUnknown, err)
	}

	// TODO -> consider sending back an ack message so the client can determin if
	// it would like to reconnect and conntinue to syncronize it's running state, or
	// be removed from the system
	res := connect.NewResponse(&sdv1.Empty{})
	res.Header().Set("blueprint-version", "v1")

	return res, nil
}

func (h *handler) Finalize(ctx context.Context, req *cnt.Request[sdv1.FinalizeRequest]) (*cnt.Response[sdv1.FinalizeResponse], error) {
	return nil, nil
}

func (h *handler) Initialize(ctx context.Context, req *cnt.Request[sdv1.InitializeRequest]) (*cnt.Response[sdv1.InitializeResponse], error) {
	var (
		nonce = req.Msg.Nonce
		name  = req.Msg.Name
	)

	identity, err := h.controller.Initialize(ctx, nonce, name)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New("failed to init the new process")
	}

	return cnt.NewResponse[sdv1.InitializeResponse](&sdv1.InitializeResponse{
		ProcessIdentity: identity,
	}), nil
}

func (h *handler) QuerySystemJournal(ctx context.Context, req *cnt.Request[sdv1.JournalQueryRequest]) (*cnt.Response[sdv1.JournalQueryResponse], error) {
	return nil, nil
}
