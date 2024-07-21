package service_discovery

import (
	"context"
	"errors"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	sdConnect "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

type (
	Rpc interface {
		chassis.RPCRegistrar
		sdConnect.ServiceDiscoveryServiceHandler
	}

	rpc struct {
		controller Controller
		logger     chassis.Logger
	}
)

func NewRPC(logger chassis.Logger, controller Controller) Rpc {
	return &rpc{
		controller: controller,
		logger:     logger,
	}
}

// Implement the `RPCRegistrar` interface of draft so the `grpc` handlers are enabled
func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := sdConnect.NewServiceDiscoveryServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

var (
	ErrFailedInitialize = "failed to init the new process"
)

func (h *rpc) Initialize(
	ctx context.Context,
	req *connect.Request[sdv1.InitializeRequest],
) (*connect.Response[sdv1.InitializeResponse], error) {
	var (
		log   = h.logger.WithContext(ctx)
		nonce = req.Msg.Nonce
		name  = req.Msg.Name
	)

	identity, err := h.controller.Initialize(ctx, log, nonce, name)
	if err != nil {
		log.
			WithError(err).
			Error(ErrFailedInitialize)

		return nil, errors.New(ErrFailedInitialize)
	}

	return connect.NewResponse[sdv1.InitializeResponse](&sdv1.InitializeResponse{
		ProcessIdentity: identity,
	}), nil
}

func (h *rpc) Synchronize(
	ctx context.Context,
	stream *connect.ClientStream[sdv1.ClientDetails],
) (*connect.Response[sdv1.Empty], error) {
	var (
		log = h.logger.WithContext(ctx)
	)

	for stream.Receive() {
		h.controller.Synchronize(ctx, log, stream.Msg())
	}

	// TODO -> handle errors
	if err := stream.Err(); err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}

	// TODO -> consider sending back an ack message so the client can determine if
	// it would like to reconnect and continue to synchronize it's running state, or
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
		log = h.logger.WithContext(ctx)
	)

	if err := h.controller.Finalize(ctx, log, pid); err != nil {
		log.WithError(err)
		return nil, err
	}

	return connect.NewResponse[sdv1.FinalizeResponse](&sdv1.FinalizeResponse{
		Pid: pid,
	}), nil
}

func (h *rpc) Query(
	ctx context.Context,
	req *connect.Request[sdv1.QueryRequest],
) (*connect.Response[sdv1.QueryResponse], error) {
	return nil, errors.New("implement me")
}

func (h *rpc) ReportHealth(
	ctx context.Context,
	req *connect.Request[sdv1.ReportHealthRequest],
) (*connect.Response[sdv1.ReportHealthResponse], error) {
	return nil, errors.New("implement me")
}
