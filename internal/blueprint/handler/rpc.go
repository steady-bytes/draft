package handler

import (
	"net/http"

	c "github.com/steady-bytes/draft/blueprint/controller"

	kvConnect "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1/v1connect"
	rgConnect "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1/v1connect"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

type (
	Handler interface {
		draft.RPCRegistrar
		// rpc handler implementations
		kvConnect.KeyValueServiceHandler
	}

	handler struct {
		controller c.Blueprint
	}
)

func New(ctr c.Blueprint) Handler {
	return &handler{
		controller: ctr,
	}
}

// Implement the `RPCRegistrar` interface of draft so the `grpc` handlers are enabled
func (h *handler) RegisterRPC(server *http.ServeMux) {
	// TODO -> find out if you can chain many different server implementations
	// server.Handle(rfConnect.NewRaftServiceHandler(h))
	server.Handle(kvConnect.NewKeyValueServiceHandler(h))
	server.Handle(rgConnect.NewServiceDiscoveryServiceHandler(h))
}
