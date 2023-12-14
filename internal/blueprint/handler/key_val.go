package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	c "github.com/steady-bytes/draft/blueprint/controller"

	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1"
	apiconnect "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1/v1connect"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	KeyValueHandler interface {
		draft.RPCRegistrar
		apiconnect.KeyValueServiceHandler
	}

	handler struct {
		keyValueController c.KeyValueController
	}
)

func New(ctr c.KeyValueController) KeyValueHandler {
	return &handler{
		keyValueController: ctr,
	}
}

func (h *handler) RegisterRPC(server *http.ServeMux) (string, http.Handler) {
	return apiconnect.NewKeyValueServiceHandler(h)
}

// Set - Responds to the rpc method `Set`. The request is checked to see if it's running on the leader
// if not then an error is returned. After, the leader is validated the payload is transformed to the `CommandPayload`
// and then apply'ed to the raft log. If that is successful then it's considered committed to the cluster.
func (h *handler) Set(ctx context.Context, req *connect.Request[kvv1.SetRequest]) (*connect.Response[kvv1.SetResponse], error) {
	var (
		key   = strings.TrimSpace(req.Msg.GetKey())
		value = req.Msg.GetValue()
	)

	fmt.Println("req: ", key)

	payload := &c.CommandPayload{
		Operation: c.Set,
		Key:       key,
		Value:     value,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	_, err = c.KeyValueController.Set(h.keyValueController, data, 500*time.Millisecond)
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
		key    = strings.TrimSpace(req.Msg.GetKey())
		filter = req.Msg.GetFilter()
	)

	value, err := h.keyValueController.Get(key)
	if err != nil {
		fmt.Println("error reading: ", err)
		return nil, errors.New("failed to get value for key")
	}

	res := &kvv1.GetResponse{}
	if filter == *kvv1.GetFilter_STRING_GET_FILTER.Enum() {
		res.Response = &kvv1.GetResponse_AsString{
			AsString: string(value),
		}
	} else {
		res.Response = &kvv1.GetResponse_AsBytes{
			AsBytes: value,
		}
	}

	return connect.NewResponse[kvv1.GetResponse](res), nil
}

func (h *handler) Delete(ctx context.Context, req *connect.Request[kvv1.DeleteRequest]) (*connect.Response[kvv1.DeleteResponse], error) {
	return nil, errors.New("not implemented")
}
