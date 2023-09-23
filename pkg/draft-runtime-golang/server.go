package draft_runtime

import (
	"fmt"
	"net"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

type ServerPluginRegistrar interface {
	IsRpc() bool

	IsHttp() bool

	// RegisterRPC - returns a `grpc.Server` after the concrete implementation has been registered with the grpc registrar.
	// The returned `grpc.Server` can then be used to run the implementation.
	RegisterRPC() *grpc.Server
	// RegisterHTTP - returns a `*gin.Engine` this gives the plugin service the opportunity to configure the router anyway needed
	// for example adding middleware and or configuring http routing
	RegisterHTTP() *gin.Engine
}

func (c *DraftRuntime) withRpc(registrar ServerPluginRegistrar) {
	var err error
	// If the builder has not already created a tcp connection then go ahead and start that now
	if c.tcp == nil {
		c.tcp, err = net.Listen("tcp", fmt.Sprintf(":%d", c.config.Service.RPCPort))
		if err != nil {
			panic(err)
		}
	}

	c.rpc = registrar.RegisterRPC()
}

func (c *DraftRuntime) withHttp(registrar ServerPluginRegistrar) {
	c.http = registrar.RegisterHTTP()
}
