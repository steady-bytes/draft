package draft_runtime_golang

import (
	"fmt"
	"net"

	ginzerolog "github.com/dn365/gin-zerolog"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

type HTTPHandlerKind int

const (
	NullHTTPHandlerKind HTTPHandlerKind = iota
	Gin
	Fiber
)

type HTTPRegistrar interface {
	// RegisterHTTP - returns a `*gin.Engine` this gives the plugin service the opportunity to configure the router anyway needed
	// for example adding middleware and or configuring http routing
	RegisterHTTP() *gin.Engine
}

func (c *Runtime) withHTTPHandler(plugin HTTPRegistrar) {
	c.withHTTPGin(plugin)
}

func (c *Runtime) withHTTPGin(registrar HTTPRegistrar) {
	c.http = registrar.RegisterHTTP()
	// add zeo-logger to gin via middleware
	c.http.Use(ginzerolog.Logger("gin"))
}

func (c *Runtime) withHTTPFiber(registrar HTTPRegistrar) {
	panic("fiber http router is not fully implemented")
}

type RPCHandlerKind int

const (
	NullRPCHandlerKind RPCHandlerKind = iota
	Grpc
)

type RPCRegistrar interface {
	// RegisterRPC - returns a `grpc.Server` after the concrete implementation has been registered with the grpc registrar.
	// The returned `grpc.Server` can then be used to run the implementation.
	RegisterRPC(server *grpc.Server)
}

func (c *Runtime) withRPCHandler(plugin RPCRegistrar) {
	c.withRpc(plugin)
}

func (c *Runtime) withRpc(registrar RPCRegistrar) {
	var err error

	c.rpc = grpc.NewServer()

	// If the builder has not already created a tcp connection then go ahead and start that now
	if c.tcp == nil {
		c.tcp, err = net.Listen("tcp", fmt.Sprintf(":%d", c.config.Service.Port))
		if err != nil {
			panic(err)
		}
	}

	registrar.RegisterRPC(c.rpc)
}
