package chassis

import (
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type RPCHandlerKind int

const (
	NullRPCHandlerKind RPCHandlerKind = iota
	Grpc
)

const (
	grpcKeepaliveTime        = 30 * time.Second
	grpcKeepaliveTimeout     = 5 * time.Second
	grpcKeepaliveMinTime     = 30 * time.Second
	grpcMaxConcurrentStreams = 1000000
)

type (
	RPCRegistrar interface {
		RegisterRPC(server Rpcer)
	}

	Rpcer interface {
		AddHandler(pattern string, handler http.Handler, enableReflection bool)
		GetGrpcServer() *grpc.Server
	}

	rpcServer struct {
		mux                    *http.ServeMux
		grpc                   *grpc.Server
		reflectionServiceNames []string
		// serviceNames are added to the `Synchronize` messages in metadata so they can be looked up by other services
		serviceNames []string
	}
)

func (c *Runtime) withRpc(registrar RPCRegistrar) {
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	c.isRPC = true

	server := &rpcServer{
		mux:                    c.mux,
		reflectionServiceNames: make([]string, 0),
	}

	registrar.RegisterRPC(server)

	if len(server.reflectionServiceNames) > 0 {
		c.rpcReflectionServiceNames = append(c.rpcReflectionServiceNames, server.reflectionServiceNames...)
	}

	if len(server.serviceNames) > 0 {
		c.rpcServiceNames = append(c.rpcServiceNames, server.serviceNames...)
	}
}

// AddHandler will register an http handler with a specific pattern to the internal mux server:
//
// If you are registering a ConnectRPC server, you can simply call the generated v1connect.NewXXXServiceHandler() to retrieve both the pattern
// and handler to pass to this method.
//
// If you are registering a gRPC server, you can pass in the service name (from the service desc) as the pattern and use the grpc.Server itself
// as the handler.
func (r *rpcServer) AddHandler(pattern string, handler http.Handler, enableReflection bool) {
	r.mux.Handle(pattern, handler)
	pattern = strings.TrimPrefix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")

	if enableReflection {
		// ConnectRPC adds some slashes for routing but they're not needed for the service naame
		r.reflectionServiceNames = append(r.reflectionServiceNames, pattern)
	}

	r.serviceNames = append(r.serviceNames, pattern)
}

// AddGRPC

func (r *rpcServer) GetGrpcServer() *grpc.Server {
	if r.grpc == nil {
		r.setupGrpcServer()
	}

	return r.grpc
}

func (r *rpcServer) setupGrpcServer() {
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions,
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    grpcKeepaliveTime,
			Timeout: grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcKeepaliveMinTime,
			PermitWithoutStream: true,
		}),
	)
	r.grpc = grpc.NewServer(grpcOptions...)
}
