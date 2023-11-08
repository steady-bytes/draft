package draft_runtime_golang

import (
	"net"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
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

	gorm     *gorm.DB
	bun      *bun.DB
	repoKind RepoKind

	rpc  *grpc.Server
	nats *nats.Conn
	http *gin.Engine

	plugin Default
}

func New(name string) *Runtime {
	rt := &Runtime{
		config: NewConfig(name),
		bun:    nil,
		rpc:    nil,
		tcp:    nil,
		http:   nil,
	}

	return rt
}

// Default - An interface that can be implemented by a service to register a `Repo`, `Rpc` interface, and a `Consumer`.
// This is kind of like the kitchen sink interface for services that have many different requirement.
type Default interface {
	RepoRegistrar
	HTTPRegistrar
	RPCRegistrar
	BrokerRegistrar
}

// DefaultRpcPlugin - Is used to register the plugin with the Runtime runtime. Runtime will save off an reference to the plugin interface for
// each bootstrapping. This is generally the first method that is called with the `Runtime`.
// func (c *Runtime) Build() *Runtime {
// 	if c.repoKind != NullRepoType {
// 		c.withRepo(c.plugin)
// 	}

// 	c.withRpc(c.plugin)
// 	c.withHttp(c.plugin)
// 	c.withBroker(c.plugin)

// 	return c
// }

func (c *Runtime) WithRepo(kind RepoKind, plugin RepoRegistrar) *Runtime {
	c.withRepo(kind, plugin)
	return c
}

func (c *Runtime) WithHTTPHandler(kind HTTPHandlerKind, plugin HTTPRegistrar) *Runtime {
	c.withHTTPHandler(kind, plugin)
	return c
}

func (c *Runtime) WithRPCHandler(kind RPCHandlerKind, plugin RPCRegistrar) *Runtime {
	c.withRPCHandler(kind, plugin)
	return c
}

type DefaultRuntimeBuilder struct{}

func (d *DefaultRuntimeBuilder) SetRepoType() RepoKind {
	return PostgresGorm
}

func (d *DefaultRuntimeBuilder) RegisterRepo(db interface{}) error {
	return nil
}

func (d *DefaultRuntimeBuilder) RegisterRPC() *grpc.Server {
	return nil
}

func (d *DefaultRuntimeBuilder) RegisterHTTP() *gin.Engine {
	return nil
}

func (d *DefaultRuntimeBuilder) SetBrokerType() BrokerType {
	return Nats
}

func (d *DefaultRuntimeBuilder) RegisterBroker(broker interface{}) error {
	return nil
}

// Start the runtime of the service. This will do things like fire up the grpc/http servers and put them on a background routine's
// TODO -> figure out how to run grpc + http on the same port
// TODO -> figure out how to run everything on a background thread so the runtime can be shutdown
func (c *Runtime) Start() error {

	if c.http != nil {
		// port := fmt.Sprintf(":%d", c.config.Service.Port)
		if err := c.http.Run(); err != nil {
			log.Panic().Msg("failed to start http service")
		}
	}

	// if c.rpc != nil {
	// 	fmt.Println("starting")
	// 	c.rpc.Serve(c.tcp)
	// 	fmt.Println("started this will never be called")
	// }

	return nil
}

func (c *Runtime) Stop() {}
