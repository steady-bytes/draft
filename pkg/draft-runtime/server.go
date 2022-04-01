package commet

import (
	"fmt"
	"net"

	fiber "github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

// RPC - Remote procedure call framework, currently the only supported framework is `gRPC` and `Protocol Buffer`'s.

type ServerPluginRegistrar interface {
	IsRpc() bool

	IsHttp() bool

	// RegisterRPC - returns a `grpc.Server` after the concrete implementation has been registered with the grpc regisrar. The returned `grpc.Server`
	// can then be used to run the implementation.
	RegisterRPC() *grpc.Server

	RegisterHTTP() *fiber.App
}

func (c *Commet) withRpc(registrar ServerPluginRegistrar) {
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

func (c *Commet) withHttp(registrar ServerPluginRegistrar) {
	c.http = registrar.RegisterHTTP()
}
