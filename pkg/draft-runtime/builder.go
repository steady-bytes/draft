package commet

import (
	"net"

	"github.com/jinzhu/gorm"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// TODO -> add logger with json formatting
// TODO -> add health check on background thread and tie it to readiness, and health checks
// TODO -> implement a graceful shutdown process
// TODO -> add ssl support on postgres

type Commet struct {
	config *Config

	tcp  net.Listener
	gorm *gorm.DB
	rpc  *grpc.Server
	nats *nats.Conn

	defaultPlugin DefaultPluginRegistrar

	rpcPlugin RpcPluginRegistrar

	aggregatePlugin AggregatePluginRegistrar
}

func New(config *Config) (*Commet, error) {
	return &Commet{
		config: config,
		gorm:   nil,
		rpc:    nil,
		tcp:    nil,
	}, nil
}

// DefaultPluginRegistrar - An interface that can be implemented by a service to register a `Repo`, `Rpc` interface, and a `Consumer`.
// This is kind of like the kictchen sink interface for services that have many different requirments.
type DefaultPluginRegistrar interface {
	RepoPluginRegistrar
	RpcPluginRegistrar
	BrokerPluginRegistrar
}

// DefaultRpcPlugin - Is used to reigister the plugin with the commet runtime. Commet will save off an refernce to the plugin interface for
// each bootstrapping. This is generally the first method that is called with the `Runtime`.
func (c *Commet) DefaultBuilder(plugin DefaultPluginRegistrar) *Commet {
	c.defaultPlugin = plugin

	if c.defaultPlugin.GetRepoType() != NullRepoType {
		c.withRepo()
	}

	if c.defaultPlugin.GetIsRpc() {
		c.withRpc()
	}

	if c.defaultPlugin.GetBrokerType() == Nats {
		c.withBroker()
	}

	return c
}

// RpcBuilder - Is used to register an rpc only process to the draft runtime.
// Prcesses like `writers`, and `readers` are usually gateways to different process that may
// expose a public read, or writer method.
func (c *Commet) RpcBuilder(plugin RpcPluginRegistrar) *Commet {
	c.rpcPlugin = plugin

	if c.rpcPlugin.GetIsRpc() {
		c.withRpc()
	}

	return c
}

// AggregatePluginRegistrar - Is the most common type of the system. It contains a repo, and rpc interface. It's
// used for simple writes, and reads to specific aggregates types.
type AggregatePluginRegistrar interface {
	RepoPluginRegistrar
	RpcPluginRegistrar
}

// AggregateBuilder - A method for building the `Aggregate` process type.
func (c *Commet) AggregateBuilder(plugin AggregatePluginRegistrar) *Commet {
	c.aggregatePlugin = plugin

	if c.aggregatePlugin.GetRepoType() != NullRepoType {
		c.withRepo()
	}

	if c.aggregatePlugin.GetIsRpc() {
		c.withRpc()
	}

	return c
}

// Start the runtime of the service. This will do things like fire up the grpc/http servers and put them on a background routine's
func (c *Commet) Start() error {
	if c.rpc != nil {

		c.rpc.Serve(c.tcp)
	}
	return nil
}

func (c *Commet) Stop() {}
