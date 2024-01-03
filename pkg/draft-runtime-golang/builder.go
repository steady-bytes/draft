package draft_runtime_golang

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
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

// Plugin Register Functions

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

// State Providers

// Init a connection to the `SecretStore`, load the secrets into memory, and pass
// the storage interface up to the runtime for use in the service
func (c *Runtime) WithSecretStore(setter SecretStoreSetter) *Runtime {
	c.withSecretStore(setter)
	return c
}

// ==============================
// DEFAULT BUILDER IMPLEMENTATION
// ==============================

// TODO -> REVIEW THE DEFAULT IMPLEMENTATION WHEN THE `Default` registrar is complete

type DefaultRuntimeBuilder struct{}

func (d *DefaultRuntimeBuilder) SetRepoType() RepoKind {
	return PostgresGORM
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
	// TODO -> configure these a little better
	corsHandler := cors.New(cors.Options{
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
		},
		AllowedOrigins: []string{"*"},
		AllowedHeaders: []string{
			"Accept-Encoding",
			"Content-Encoding",
			"Content-Type",
			"Connect-Protocol-Version",
			"Connect-Timeout-Ms",
			"Connect-Accept-Encoding",  // Unused in web browsers, but added for future-proofing
			"Connect-Content-Encoding", // Unused in web browsers, but added for future-proofing
			"Grpc-Timeout",             // Used for gRPC-web
			"X-Grpc-Web",               // Used for gRPC-web
			"X-User-Agent",             // Used for gRPC-web
		},
		ExposedHeaders: []string{
			"Content-Encoding",         // Unused in web browsers, but added for future-proofing
			"Connect-Content-Encoding", // Unused in web browsers, but added for future-proofing
			"Grpc-Status",              // Required for gRPC-web
			"Grpc-Message",             // Required for gRPC-web
		},
	})

	if c.http != nil {
		if err := c.http.Run(); err != nil {
			log.Panic().Msg("failed to start http service")
		}
	}

	if c.rpc != nil {
		// c.rpcServer.Serve(c.tcp)
		handler := corsHandler.Handler(c.rpc)
		if err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", c.config.Service.Port), h2c.NewHandler(handler, &http2.Server{})); err != nil {
			fmt.Println(err)
		}
	}

	return nil
}

func (c *Runtime) Stop() {}
