package chassis

import (
	"net/http"
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
		EnableReflection(string)
		IsReflection() bool
		AddHandler(string, http.Handler)
		GetGrpcServer() *grpc.Server
		Logger() Logger
	}

	rpcServer struct {
		mux            *http.ServeMux
		grpc 		   *grpc.Server
		rpcServiceName string
		isReflection   bool
		logger         Logger
	}
)

func (c *Runtime) withRpc(registrar RPCRegistrar) {
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	c.isRPC = true

	server := &rpcServer{
		mux:          c.mux,
		isReflection: false,
		logger:       c.logger,
	}

	registrar.RegisterRPC(server)

	if server.IsReflection() {
		c.rpcReflectionServiceNames = append(c.rpcReflectionServiceNames, server.rpcServiceName)
	}
}

func (r *rpcServer) EnableReflection(serviceName string) {
	r.isReflection = true
	r.rpcServiceName = serviceName
}

func (r *rpcServer) IsReflection() bool {
	return r.isReflection
}

func (r *rpcServer) AddHandler(name string, handler http.Handler) {
	r.mux.Handle(name, handler)
}

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

func (r *rpcServer) Logger() Logger {
	return r.logger
}
