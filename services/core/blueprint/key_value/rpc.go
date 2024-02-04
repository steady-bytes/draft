package key_value

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	kvv1 "github.com/steady-bytes/draft/api/registry/key_value/v1"
	kvConnect "github.com/steady-bytes/draft/api/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	draft "github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/logging"
)

type (
	Rpc interface {
		draft.RPCRegistrar

		kvConnect.KeyValueServiceHandler
	}

	rpc struct {
		controller Controller
		logger     logging.Logger
	}
)

func New(controller Controller) Rpc {
	return &rpc{
		controller: controller,
		logger:     nil,
	}
}

var (
	ErrFailedSet  = errors.New("failed to set key/value pair")
	ErrFailedList = errors.New("failed to list all values for provided kind")
)

func (h *rpc) RegisterRPC(server draft.Rpcer) {
	server.EnableReflection(kvConnect.KeyValueServiceName)
	server.AddHandler(kvConnect.NewKeyValueServiceHandler(h))
	h.logger = server.Logger()
}

func (h *rpc) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
	var (
		log   = h.logger.WithContext(ctx)
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
		err   error
	)

	_, err = h.controller.Set(log, key, value, 500*time.Millisecond)
	if err != nil {
		log.
			WithError(err).
			Error(ErrFailedSet.Error())

		return nil, err
	}

	log.WithField("key", key).Info("value saved")

	return connect.NewResponse[kvv1.SetResponse](&kvv1.SetResponse{
		Key: key,
	}), nil
}

func (h *rpc) Get(ctx context.Context, req *connect.Request[kvv1.GetRequest]) (*connect.Response[kvv1.GetResponse], error) {
	var (
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
	)

	value, err := h.controller.Get(key, value)
	if err != nil {
		fmt.Println("error reading: ", err)
		return nil, errors.New("failed to get value for key")
	}

	res := &kvv1.GetResponse{
		Value: value,
	}

	return connect.NewResponse[kvv1.GetResponse](res), nil
}

func (h *rpc) Delete(ctx context.Context, req *connect.Request[kvv1.DeleteRequest]) (*connect.Response[kvv1.DeleteResponse], error) {
	return nil, errors.New("not implemented")
}

func (h *rpc) List(ctx context.Context, req *connect.Request[kvv1.ListRequest]) (*connect.Response[kvv1.ListResponse], error) {
	var (
		kind = req.Msg.GetValue()
	)

	valuesMap, err := h.controller.List(kind)
	if err != nil {
		fmt.Println(err)
		return nil, ErrFailedList
	}

	fmt.Println(valuesMap)

	res := &kvv1.ListResponse{
		Values: valuesMap,
	}

	return connect.NewResponse[kvv1.ListResponse](res), nil
}
