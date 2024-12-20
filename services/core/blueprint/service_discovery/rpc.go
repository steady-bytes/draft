package service_discovery

import (
	"context"
	"errors"
	"io"

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

func (h *rpc) Initialize(ctx context.Context, req *connect.Request[sdv1.InitializeRequest]) (*connect.Response[sdv1.InitializeResponse], error) {
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

func (h *rpc) Synchronize(ctx context.Context, stream *connect.BidiStream[sdv1.ClientDetails, sdv1.ClusterDetails]) error {
	var (
		log = h.logger.WithContext(ctx)
		id  = ""
	)

	for {
		res, err := stream.Receive()
		if err != nil && errors.Is(err, io.EOF) {
			// remove from system journal
			h.controller.Finalize(ctx, log, id)
			return nil
		} else if err != nil {
			// TODO: determine how to handle this error
			log.WithError(err).Error("connection error")
			return err
		}

		id = res.Pid

		if err := ctx.Err(); err != nil {
			h.controller.Finalize(ctx, log, id)
			return err
		}

		h.controller.Synchronize(ctx, log, res)

		// when an update packet is received from the service and the connection is still live
		// send blueprint cluster details down to the client
		details := h.controller.GetClusterDetails()
		if err := stream.Send(details); err != nil {
			log.WithError(err).Error("failed to send cluster details to the client")
			return err
		}
	}
}

func (h *rpc) Finalize(ctx context.Context, req *connect.Request[sdv1.FinalizeRequest]) (*connect.Response[sdv1.FinalizeResponse], error) {
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

func (h *rpc) Query(ctx context.Context, req *connect.Request[sdv1.QueryRequest]) (*connect.Response[sdv1.QueryResponse], error) {
	res, err := h.controller.Query(ctx, h.logger.WithContext(ctx))
	if err != nil {
		h.logger.WithError(err).Error(err.Error())
		return nil, err
	}

	return connect.NewResponse[sdv1.QueryResponse](&sdv1.QueryResponse{
		Data: res,
	}), nil
}

func (h *rpc) ReportHealth(
	ctx context.Context,
	req *connect.Request[sdv1.ReportHealthRequest],
) (*connect.Response[sdv1.ReportHealthResponse], error) {
	return nil, errors.New("implement me")
}
