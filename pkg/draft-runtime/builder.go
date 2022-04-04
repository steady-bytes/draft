package draft_runtime

import (
	"fmt"
	"net"

	fiber "github.com/gofiber/fiber/v2"
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
	http *fiber.App

	defaultPlugin DefaultPluginRegistrar
}

func New(config *Config) (*Commet, error) {
	return &Commet{
		config: config,
		gorm:   nil,
		rpc:    nil,
		tcp:    nil,
		http:   nil,
	}, nil
}

// DefaultPluginRegistrar - An interface that can be implemented by a service to register a `Repo`, `Rpc` interface, and a `Consumer`.
// This is kind of like the kictchen sink interface for services that have many different requirments.
type DefaultPluginRegistrar interface {
	RepoPluginRegistrar
	ServerPluginRegistrar
	BrokerPluginRegistrar
}

// DefaultRpcPlugin - Is used to reigister the plugin with the commet runtime. Commet will save off an refernce to the plugin interface for
// each bootstrapping. This is generally the first method that is called with the `Runtime`.
func (c *Commet) DefaultBuilder(plugin DefaultPluginRegistrar) *Commet {
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

func (d *DefaultRuntimeBuilder) RegisterHTTP() *fiber.App {
	return nil
}

func (d *DefaultRuntimeBuilder) GetBrokerType() BrokerType {
	return Nats
}

func (d *DefaultRuntimeBuilder) RegisterBroker(broker interface{}) error {
	return nil
}

// Start the runtime of the service. This will do things like fire up the grpc/http servers and put them on a background routine's
func (c *Commet) Start() error {
	fmt.Println("start called")

	if c.http != nil {
		port := fmt.Sprintf(":%d", c.config.Service.HTTPPort)
		go c.http.Listen(port)
	}

	if c.rpc != nil {
		fmt.Println("starting")
		c.rpc.Serve(c.tcp)
		fmt.Println("started this will never be called")
	}

	return nil
}

func (c *Commet) Stop() {}
