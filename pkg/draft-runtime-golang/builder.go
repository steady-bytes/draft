package draft_runtime_golang

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// Default - An interface that can be implemented by a service to register a `Repo`, `Rpc` interface, and a `Consumer`.
// This is kind of like the kitchen sink interface for services that have many different requirement.
type Default interface {
	RepoRegistrar
	HTTPRegistrar
	RPCRegistrar
	BrokerRegistrar
	ConsensusRegistrar
}

func (c *Runtime) WithRepo(kind RepoKind, plugin RepoRegistrar) *Runtime {
	c.withRepo(kind, plugin)
	return c
}

func (c *Runtime) WithHTTPHandler(plugin HTTPRegistrar) *Runtime {
	c.withHTTPHandler(plugin)
	return c
}

func (c *Runtime) WithRPCHandler(plugin RPCRegistrar) *Runtime {
	c.withRPCHandler(plugin)
	return c
}

func (c *Runtime) WithConsensus(kind ConsensusKind, plugin ConsensusRegistrar) *Runtime {
	c.withConsensus(kind, plugin)
	return c
}

// ==============================
// DEFAULT BUILDER IMPLEMENTATION
// ==============================

// TODO -> REVIEW THE DEFAULT IMPLEMENTATION WHEN THE `Default` registrar is complete

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
		if err := c.http.Run(); err != nil {
			log.Panic().Msg("failed to start http service")
		}
	}

	if c.rpc != nil {
		c.rpc.Serve(c.tcp)
	}

	return nil
}

func (c *Runtime) Stop() {}
