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
	ErrFailedSet    = errors.New("failed to set key/value pair")
	ErrFailedList   = errors.New("failed to list all values for provided kind")
	ErrFailedDelete = errors.New("failed to delete by key for provided kind")
	ErrFailedGet    = errors.New("failed to get value for key")
)

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := kvConnect.NewKeyValueServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

func (h *rpc) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
	var (
		log   = h.logger.WithContext(ctx)
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
	)

	_, err := h.controller.Set(log, key, value, 500*time.Millisecond)
	if err != nil {
		log.WithError(err).Error(ErrFailedSet.Error())
		return nil, ErrFailedSet
	}

	log.WithField("key", key).Debug("value saved")

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
		return nil, ErrFailedGet
	}

	return connect.NewResponse(&kvv1.GetResponse{
		Value: value,
	}), nil
}

func (h *rpc) Delete(ctx context.Context, req *connect.Request[kvv1.DeleteRequest]) (*connect.Response[kvv1.DeleteResponse], error) {
	var (
		log   = h.logger.WithContext(ctx)
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
	)

	err := h.controller.Delete(log, key, value)
	if err != nil {
		h.logger.WithError(err).Error(ErrFailedDelete.Error())
		return nil, ErrFailedDelete
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
		return nil, ErrFailedList
	}

	return connect.NewResponse(&kvv1.ListResponse{
		Values: valuesMap,
	}), nil
}
