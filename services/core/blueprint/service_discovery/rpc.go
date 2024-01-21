package service_discovery

import (
	"context"
	"errors"
	"fmt"

	sdv1 "github.com/steady-bytes/draft/api/registry/service_discovery/v1"
	sdConnect "github.com/steady-bytes/draft/api/registry/service_discovery/v1/v1connect"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"connectrpc.com/connect"
)

type (
	Rpc interface {
		draft.RPCRegistrar
		sdConnect.ServiceDiscoveryServiceHandler
	}

	rpc struct {
		controller Controller
	}
)

func New(controller Controller) Rpc {
	return &rpc{
		controller: controller,
	}
}

// Implement the `RPCRegistrar` interface of draft so the `grpc` handlers are enabled
func (h *rpc) RegisterRPC(server draft.Rpcer) {
	server.EnableReflection(sdConnect.ServiceDiscoveryServiceName)
	server.AddHandler(sdConnect.NewServiceDiscoveryServiceHandler(h))
}

func (h *rpc) Synchronize(
	ctx context.Context,
	stream *connect.ClientStream[sdv1.ClientDetails],
) (*connect.Response[sdv1.Empty], error) {
	for stream.Receive() {
		h.controller.Synchronize(ctx, stream.Msg())
	}

	// TODO -> handle errors
	if err := stream.Err(); err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}

	// TODO -> consider sending back an ack message so the client can determin if
	// it would like to reconnect and conntinue to syncronize it's running state, or
	// be removed from the system
	res := connect.NewResponse(&sdv1.Empty{})
	res.Header().Set("blueprint-version", "v1")

	return res, nil
}

func (h *rpc) Finalize(
	ctx context.Context,
	req *connect.Request[sdv1.FinalizeRequest],
) (*connect.Response[sdv1.FinalizeResponse], error) {
	var (
		pid = req.Msg.Pid
	)

	if err := h.controller.Finalize(ctx, pid); err != nil {
		fmt.Println(err)
		return nil, err
	}

	return connect.NewResponse[sdv1.FinalizeResponse](&sdv1.FinalizeResponse{
		Pid: pid,
	}), nil
}

func (h *rpc) Initialize(
	ctx context.Context,
	req *connect.Request[sdv1.InitializeRequest],
) (*connect.Response[sdv1.InitializeResponse], error) {
	var (
		nonce = req.Msg.Nonce
		name  = req.Msg.Name
	)

	identity, err := h.controller.Initialize(ctx, nonce, name)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New("failed to init the new process")
	}

	return connect.NewResponse[sdv1.InitializeResponse](&sdv1.InitializeResponse{
		ProcessIdentity: identity,
	}), nil
}

func (h *rpc) QuerySystemJournal(
	ctx context.Context,
	req *connect.Request[sdv1.JournalQueryRequest],
) (*connect.Response[sdv1.JournalQueryResponse], error) {
	// h.controller.Query(ctx)
	return nil, errors.New("implement me")
}
