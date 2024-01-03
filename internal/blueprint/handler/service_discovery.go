package handler

import (
	"context"

	cnt "connectrpc.com/connect"
	sdv1 "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1"
)

func (h *handler) Connect(ctx context.Context, req *cnt.ClientStream[sdv1.ProcessDetails]) (*cnt.Response[sdv1.Empty], error) {
	return nil, nil
}

func (h *handler) Disconnect(ctx context.Context, req *cnt.Request[sdv1.DisconnectRequest]) (*cnt.Response[sdv1.DisconnectResponse], error) {
	return nil, nil
}

func (h *handler) Init(ctx context.Context, req *cnt.Request[sdv1.InitRequest]) (*cnt.Response[sdv1.InitResponse], error) {
	var (
		nonce = req.Msg.Nonce
		name  = req.Msg.Name
	)

	identity, err := h.controller.Init(ctx, nonce, name)
	if err != nil {
	}

	return cnt.NewResponse[sdv1.InitResponse](&sdv1.InitResponse{
		ProcessIdentity: identity,
	}), nil
}

func (h *handler) QuerySystemJournal(ctx context.Context, req *cnt.Request[sdv1.JournalQueryRequest]) (*cnt.Response[sdv1.JournalQueryResponse], error) {
	return nil, nil
}
