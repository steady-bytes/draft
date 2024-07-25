package key_value

import (
	"context"
	"errors"
	"fmt"
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
	ErrFailedSet  = errors.New("failed to set key/value pair")
	ErrFailedList = errors.New("failed to list all values for provided kind")
)

func (h *rpc) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := kvConnect.NewKeyValueServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

func (h *rpc) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
	var (
		logger = h.logger.WithContext(ctx)
		key    = strings.TrimSpace(req.Msg.GetKey())
		value  = req.Msg.GetValue()
		err    error
	)

	_, err = h.controller.Set(logger, key, value, 500*time.Millisecond)
	if err != nil {
		logger.
			WithError(err).
			Error(ErrFailedSet.Error())

		return nil, err
	}

	logger.WithField("key", key).Info("value saved")

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
		fmt.Println("error reading: ", err)
		return nil, errors.New("failed to get value for key")
	}

	return connect.NewResponse(&kvv1.GetResponse{
		Value: value,
	}), nil
}

func (h *rpc) Delete(ctx context.Context, req *connect.Request[kvv1.DeleteRequest]) (*connect.Response[kvv1.DeleteResponse], error) {
	return nil, errors.New("not implemented")
}

func (h *rpc) List(ctx context.Context, req *connect.Request[kvv1.ListRequest]) (*connect.Response[kvv1.ListResponse], error) {
	var (
		logger = h.logger.WithContext(ctx)
		kind   = req.Msg.GetValue()
	)

	valuesMap, err := h.controller.List(logger, kind)
	if err != nil {
		fmt.Println(err)
		return nil, ErrFailedList
	}

	return connect.NewResponse(&kvv1.ListResponse{
		Values: valuesMap,
	}), nil
}
