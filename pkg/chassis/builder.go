package chassis

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"connectrpc.com/grpcreflect"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

type (
	// CloseChan is used to signal to the runtime that all servers, and connections need
	// to be closed down.
	CloseChan = chan os.Signal
	Default   interface {
		RepoRegistrar
		HTTPRegistrar
		RPCRegistrar
		BrokerRegistrar
		ConsensusRegistrar
	}
)

////////////////////////////
// Plugin Register Functions
////////////////////////////

func (c *Runtime) WithRepo(kind RepoKind, plugin RepoRegistrar) *Runtime {
	c.withRepo(kind, plugin)
	return c
}

func (c *Runtime) WithHTTPHandler(kind HTTPKind, plugin HTTPRegistrar) *Runtime {
	c.withHTTPHandler(kind, plugin)
	return c
}

func (c *Runtime) WithRPCHandler(plugin RPCRegistrar) *Runtime {
	c.withRpc(plugin)
	return c
}

func (c *Runtime) WithConsensus(kind ConsensusKind, plugin ConsensusRegistrar) *Runtime {
	c.withConsensus(kind, plugin)
	return c
}

//////////////////
// State Providers
//////////////////

// Init a connection to the `SecretStore`, load the secrets into memory, and pass
// the storage interface up to the runtime for use in the service
func (c *Runtime) UseSecretStore(setter SecretStoreSetter) *Runtime {
	c.withSecretStore(setter)
	return c
}

////////////////////
// Runtime Functions
////////////////////

// Start the runtime of the service. This will do things like fire up the grpc/http servers and put
// them on a background routine's
func (c *Runtime) Start() error {
	cors := c.buildCors()
	handler := cors.Handler(c.mux)

	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	close := make(chan os.Signal, 1)
	signal.Notify(close, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	if c.isHTTP == true {
		go c.runHTTP(close, handler)
	}

	if c.isRPC == true {
		go c.runRPC(close, handler)
	}

	// forever loop that runs until `close` signal is received
	for {
		select {
		case <-close:
			fmt.Println("close signal received")
			os.Exit(0)
		}
	}
}

func (c *Runtime) runHTTP(close CloseChan, handler http.Handler) {
	if err := http.ListenAndServe(c.config.Service.GetAddress(), handler); err != nil {
		fmt.Println(err)
	}
}

func (c *Runtime) runRPC(close CloseChan, handler http.Handler) {
	if len(c.rpcReflectionServiceNames) > 0 {
		reflector := grpcreflect.NewStaticReflector(c.rpcReflectionServiceNames...)
		c.mux.Handle(grpcreflect.NewHandlerV1(reflector))
		c.mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}

	if err := http.ListenAndServe(c.config.Service.GetAddress(), h2c.NewHandler(handler, &http2.Server{})); err != nil {
		fmt.Println(err)
	}
}

func (c *Runtime) buildCors() *cors.Cors {
	return cors.New(cors.Options{
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
}
