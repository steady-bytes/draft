package draft_runtime_golang

import (
	"fmt"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"github.com/nats-io/nats.go"
	"github.com/uptrace/bun"
	"google.golang.org/grpc"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// TODO -> add logger with json formatting
// TODO -> add health check on background thread and tie it to readiness, and health checks
// TODO -> implement a graceful shutdown process
// TODO -> add ssl support on postgres

type Runtime struct {
	config *Config

	tcp net.Listener

	gorm *gorm.DB
	bun  *bun.DB

	rpc  *grpc.Server
	nats *nats.Conn
	http *gin.Engine

	defaultPlugin DefaultPluginRegistrar
}

func New(config *Config) (*Runtime, error) {
	return &Runtime{
		config: config,
		gorm:   nil,
		rpc:    nil,
		tcp:    nil,
		http:   nil,
	}, nil
}

// DefaultPluginRegistrar - An interface that can be implemented by a service to register a `Repo`, `Rpc` interface, and a `Consumer`.
// This is kind of like the kitchen sink interface for services that have many different requirement.
type DefaultPluginRegistrar interface {
	RepoPluginRegistrar
	ServerPluginRegistrar
	BrokerPluginRegistrar
}

// DefaultRpcPlugin - Is used to register the plugin with the Runtime runtime. Runtime will save off an reference to the plugin interface for
// each bootstrapping. This is generally the first method that is called with the `Runtime`.
func (c *Runtime) DefaultBuilder(plugin DefaultPluginRegistrar) *Runtime {
	c.withRepo(plugin)
	c.withRpc(plugin)
	c.withHttp(plugin)
	c.withBroker(plugin)

	c.defaultPlugin = plugin

	return c
}

type DefaultRuntimeBuilder struct{}

func (d *DefaultRuntimeBuilder) GetRepoType() RepoType {
	return PostgresGorm
}

func (d *DefaultRuntimeBuilder) RegisterDB(db interface{}) error {
	return nil
}

func (d *DefaultRuntimeBuilder) IsRpc() bool {
	return false
}

func (d *DefaultRuntimeBuilder) RegisterRPC() *grpc.Server {
	return nil
}

func (s *DefaultRuntimeBuilder) IsHttp() bool {
	return false
}

func (d *DefaultRuntimeBuilder) RegisterHTTP() *gin.Engine {
	return nil
}

func (d *DefaultRuntimeBuilder) GetBrokerType() BrokerType {
	return Nats
}

func (d *DefaultRuntimeBuilder) RegisterBroker(broker interface{}) error {
	return nil
}

// Start the runtime of the service. This will do things like fire up the grpc/http servers and put them on a background routine's
func (c *Runtime) Start() error {
	fmt.Println("start called")

	/* if c.http != nil {
	 *   port := fmt.Sprintf(":%d", c.config.Service.HTTPPort)
	 *   go c.http.Listen(port)
	 * } */

	if c.rpc != nil {
		fmt.Println("starting")
		c.rpc.Serve(c.tcp)
		fmt.Println("started this will never be called")
	}

	return nil
}

func (c *Runtime) Stop() {}
