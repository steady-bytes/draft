package key_value

import (
	"context"
	"errors"
	"strings"
	"time"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvConnect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
)

type (
	Rpc interface {
		chassis.RPCRegistrar

		kvConnect.KeyValueServiceHandler
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

var (
	ErrFailedSet         = errors.New("failed to set key/value pair")
	ErrFailedList        = errors.New("failed to list all values for provided kind")
	ErrFailedListKinds   = errors.New("failed to list kinds")
	ErrFailedDelete      = errors.New("failed to delete by key for provided kind")
	ErrFailedGet         = errors.New("failed to get value for key")
	ErrInvalidDescriptor = errors.New("invalid type descriptor")
)

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := kvConnect.NewKeyValueServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

func (h *rpc) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
	var (
		callerService = chassis.CallerServiceFromHeader(req.Header())
		log           = h.logger.WithContext(ctx)
		key           = strings.TrimSpace(req.Msg.GetKey())
		value         = req.Msg.GetValue()
	)
	ctx = WithCallerService(ctx, callerService)

	_, err := h.controller.Set(ctx, log, key, value, 500*time.Millisecond)
	if err != nil {
		log.WithError(err).Error(ErrFailedSet.Error())
		return nil, connect.NewError(connect.CodeInternal, ErrFailedSet)
	}

	log.WithField("key", key).WithField("caller_service", callerService).Debug("value saved")

	return connect.NewResponse(&kvv1.SetResponse{
		Key: key,
	}), nil
}

func (h *rpc) Get(ctx context.Context, req *connect.Request[kvv1.GetRequest]) (*connect.Response[kvv1.GetResponse], error) {
	var (
		log   = h.logger.WithContext(ctx)
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
	)

	value, err := h.controller.Get(log, key, value)
	if err != nil {
		log.WithError(err).Error(ErrFailedGet.Error())
		return nil, connect.NewError(connect.CodeNotFound, ErrFailedGet)
	}

	return connect.NewResponse(&kvv1.GetResponse{
		Value: value,
	}), nil
}

func (h *rpc) Delete(ctx context.Context, req *connect.Request[kvv1.DeleteRequest]) (*connect.Response[kvv1.DeleteResponse], error) {
	var (
		callerService = chassis.CallerServiceFromHeader(req.Header())
		log           = h.logger.WithContext(ctx)
		key           = strings.TrimSpace(req.Msg.GetKey())
		value         = req.Msg.GetValue()
	)
	ctx = WithCallerService(ctx, callerService)

	err := h.controller.Delete(ctx, log, key, value, 500*time.Millisecond)
	if err != nil {
		h.logger.WithError(err).Error(ErrFailedDelete.Error())
		return nil, connect.NewError(connect.CodeInternal, ErrFailedDelete)
	}

	return connect.NewResponse(&kvv1.DeleteResponse{
		Key: key,
	}), nil
}

func (h *rpc) List(ctx context.Context, req *connect.Request[kvv1.ListRequest]) (*connect.Response[kvv1.ListResponse], error) {
	var (
		log  = h.logger.WithContext(ctx)
		kind = req.Msg.GetValue()
	)

	valuesMap, err := h.controller.List(log, kind)
	if err != nil {
		log.WithError(err).Error(ErrFailedList.Error())
		return nil, connect.NewError(connect.CodeNotFound, ErrFailedList)
	}

	return connect.NewResponse(&kvv1.ListResponse{
		Values: valuesMap,
	}), nil
}

func (h *rpc) ListKinds(ctx context.Context, req *connect.Request[kvv1.ListKindsRequest]) (*connect.Response[kvv1.ListKindsResponse], error) {
	log := h.logger.WithContext(ctx)

	kinds, err := h.controller.ListKinds(log)
	if err != nil {
		log.WithError(err).Error(ErrFailedListKinds.Error())
		return nil, connect.NewError(connect.CodeInternal, ErrFailedListKinds)
	}

	pbKinds := make([]*kvv1.KindSummary, len(kinds))
	for i, k := range kinds {
		pbKinds[i] = &kvv1.KindSummary{TypeUrl: k.TypeURL, Count: k.Count}
	}

	return connect.NewResponse(&kvv1.ListKindsResponse{
		Kinds: pbKinds,
	}), nil
}

func (h *rpc) RegisterType(ctx context.Context, req *connect.Request[kvv1.RegisterTypeRequest]) (*connect.Response[kvv1.RegisterTypeResponse], error) {
	var (
		log        = h.logger.WithContext(ctx)
		descriptor = req.Msg.GetDescriptor_()
	)

	if descriptor.GetTypeUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, ErrInvalidDescriptor)
	}

	if err := h.controller.RegisterType(ctx, log, descriptor); err != nil {
		log.WithError(err).Error(ErrFailedRegisterType.Error())
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	return connect.NewResponse(&kvv1.RegisterTypeResponse{}), nil
}

func (h *rpc) DecodeValues(ctx context.Context, req *connect.Request[kvv1.DecodeValuesRequest]) (*connect.Response[kvv1.DecodeValuesResponse], error) {
	log := h.logger.WithContext(ctx)

	decoded := h.controller.DecodeValues(log, req.Msg.GetTypeUrl(), req.Msg.GetValues())

	return connect.NewResponse(&kvv1.DecodeValuesResponse{
		Json: decoded,
	}), nil
}
