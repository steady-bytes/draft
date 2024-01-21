package key_value

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1"
	kvConnect "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1/v1connect"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	Rpc interface {
		draft.RPCRegistrar

		kvConnect.KeyValueServiceHandler
	}

	rpc struct {
		controller Controller
	}
)

func New(controller Controller) Rpc {
	return &rpc{controller}
}

func (h *rpc) RegisterRPC(server draft.Rpcer) {
	server.EnableReflection(kvConnect.KeyValueServiceName)
	server.AddHandler(kvConnect.NewKeyValueServiceHandler(h))
}

func (h *rpc) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
	var (
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
		err   error
	)

	_, err = h.controller.Set(key, value, 500*time.Millisecond)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return connect.NewResponse[kvv1.SetResponse](&kvv1.SetResponse{
		Key: key,
	}), nil
}

// Get - Looks for a key that maybe in the `Log` and if found returns the associated value
func (h *rpc) Get(ctx context.Context, req *connect.Request[kvv1.GetRequest]) (*connect.Response[kvv1.GetResponse], error) {
	var (
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
	)

	value, err := h.controller.Get(key, value.GetTypeUrl())
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

func (h *rpc) Query(ctx context.Context, req *connect.Request[kvv1.QueryRequest]) (*connect.Response[kvv1.QueryResponse], error) {
	return nil, errors.New("not implemented")
}
