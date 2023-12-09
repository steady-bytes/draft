package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	c "github.com/steady-bytes/draft/blueprint/controller"

	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_val/v1"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"google.golang.org/grpc"
)

type (
	KeyValueHandler interface {
		draft.RPCRegistrar
	}

	handler struct {
		kvv1.UnimplementedKeyValueServiceServer

		keyValueController c.KeyValueController
	}
)

func New(ctr c.KeyValueController) KeyValueHandler {
	return &handler{
		keyValueController: ctr,
	}
}

func (h *handler) RegisterRPC(server *grpc.Server) {
	kvv1.RegisterKeyValueServiceServer(server, h)
}

// Set - Responds to the rpc method `Set`. The request is checked to see if it's running on the leader
// if not then an error is returned. After, the leader is validated the payload is transformed to the `CommandPayload`
// and then apply'ed to the raft log. If that is successful then it's considered committed to the cluster.
func (h *handler) Set(ctx context.Context, req *kvv1.SetRequest) (*kvv1.SetResponse, error) {
	var (
		key = strings.TrimSpace(req.GetKey())
	)

	fmt.Println("req: ", key)

	payload := &c.CommandPayload{
		Operation: c.Set,
		Key:       key,
		Value:     req.GetValue(),
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

	return &kvv1.SetResponse{
		Key: key,
	}, nil
}

// Get - Looks for a key that maybe in the `Log` and if found returns the associated value
func (h *handler) Get(ctx context.Context, req *kvv1.GetRequest) (*kvv1.GetResponse, error) {
	var (
		key    = strings.TrimSpace(req.GetKey())
		filter = req.GetFilter()
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

	return res, nil
}
