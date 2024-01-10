package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Set - Responds to the rpc method `Set`. The request is checked to see if it's running on the leader
// if not then an error is returned. After, the leader is validated the payload is transformed to the `CommandPayload`
// and then apply'ed to the raft log. If that is successful then it's considered committed to the cluster.
func (h *handler) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
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
func (h *handler) Get(ctx context.Context, req *connect.Request[kvv1.GetRequest]) (*connect.Response[kvv1.GetResponse], error) {
	var (
		key = strings.TrimSpace(req.Msg.GetKey())
	)

	value, err := h.controller.Get(key)
	if err != nil {
		fmt.Println("error reading: ", err)
		return nil, errors.New("failed to get value for key")
	}

	any := &anypb.Any{}
	if err := anypb.MarshalFrom(any, *value, proto.MarshalOptions{}); err != nil {
		return nil, err
	}

	res := &kvv1.GetResponse{
		Value: any,
	}

	return connect.NewResponse[kvv1.GetResponse](res), nil
}

func (h *handler) Delete(ctx context.Context, req *connect.Request[kvv1.DeleteRequest]) (*connect.Response[kvv1.DeleteResponse], error) {
	return nil, errors.New("not implemented")
}

func (h *handler) Query(ctx context.Context, req *connect.Request[kvv1.QueryRequest]) (*connect.Response[kvv1.QueryResponse], error) {
	h.controller.Iterate()

	return nil, errors.New("not implemented")
}
