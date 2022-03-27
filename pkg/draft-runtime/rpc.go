package commet

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
)

// RPC - Remote procedure call framework, currently the only supported framework is `gRPC` and `Protocol Buffer`'s.

type RpcPluginRegistrar interface {
	// GetIsRpc -
	GetIsRpc() bool

	// RegisterRPC - returns a `grpc.Server` after the concrete implementation has been registered with the grpc regisrar. The returned `grpc.Server`
	// can then be used to run the implementation.
	RegisterRPC() *grpc.Server
}

func (c *Commet) withRpc() {
	var err error
	// If the builder has not already created a tcp connection then go ahead and start that now
	if c.tcp == nil {
		c.tcp, err = net.Listen("tcp", fmt.Sprintf(":%d", c.config.Service.Port))
		if err != nil {
			panic(err)
		}
	}

	if c.defaultPlugin != nil {
		// if the defaultPlugin contains the `repoType` integrated with `GROM` then it's assumed the
		// default gorm generated server implementation is being used thus, a reference to the orm
		// needs to be saved in the concret implementation
		if c.defaultPlugin.GetRepoType() == PostgresGorm {
			if err := c.defaultPlugin.RegisterDB(c.gorm); err != nil {
				panic(err)
			}
		}

		// store off the rpc server that has been registered with implementing handler interface
		c.rpc = c.defaultPlugin.RegisterRPC()
	}

	if c.rpcPlugin != nil {
		c.rpc = c.rpcPlugin.RegisterRPC()
	}
}
