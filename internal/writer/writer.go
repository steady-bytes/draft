package writer

import (
	api "github.com/steady-bytes/draft/api/gen/go"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime"

	"google.golang.org/grpc"
)

func NewPlugin() draft.RpcPluginRegistrar {
	return &writerPlugin{
		service: NewService(),
	}
}

type writerPlugin struct {
	service *service
}

// Implement the `draft.RpcPluginRegistrar` interface because the `EventStore`
// contains a `Create` event for external clients like `web-app`'s, `mobile` app's
// and native desktop applications to create and event known to the whole system
// of services.
func (s *writerPlugin) IsRpc() bool {
	return true
}

func (s *writerPlugin) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	api.RegisterWriterServer(server, s.service)

	return server
}
