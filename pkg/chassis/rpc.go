package chassis

import (
	"net/http"
)

type RPCHandlerKind int

const (
	NullRPCHandlerKind RPCHandlerKind = iota
	Grpc
)

type (
	RPCRegistrar interface {
		RegisterRPC(server Rpcer)
	}

	Rpcer interface {
		EnableReflection(string)
		IsReflection() bool
		AddHandler(string, http.Handler)
		Logger() Logger
	}

	rpcServer struct {
		mux            *http.ServeMux
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

func (r *rpcServer) Logger() Logger {
	return r.logger
}
