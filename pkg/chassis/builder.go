package chassis

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	"connectrpc.com/grpcreflect"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
)

type (
	// CloseChan is used to signal to the runtime that all servers, and connections need
	// to be closed down.
	CloseChan = chan os.Signal
	Default   interface {
		RPCRegistrar
		ConsensusRegistrar
	}
)

////////////////////////////
// Plugin Register Functions
////////////////////////////

func (c *Runtime) WithRepository(plugin Repository) *Runtime {
	logger := c.logger.WithField("plugin", reflect.TypeOf(plugin).String())
	err := plugin.Open(context.Background(), c.config)
	if err != nil {
		logger.WithError(err).Fatal("failed to set up repository plugin")
	}
	c.repositories = append(c.repositories, plugin)
	logger.Info("successfully set up repository plugin")
	return c
}

func (c *Runtime) WithBroker(plugin Broker) *Runtime {
	logger := c.logger.WithField("plugin", reflect.TypeOf(plugin).String())
	err := plugin.Open(context.Background(), c.config)
	if err != nil {
		logger.WithError(err).Fatal("failed to set up broker plugin")
	}
	c.brokers = append(c.brokers, plugin)
	logger.Info("successfully set up broker plugin")
	return c
}

func (c *Runtime) WithSecretStore(plugin SecretStore) *Runtime {
	logger := c.logger.WithField("plugin", reflect.TypeOf(plugin).String())
	err := plugin.Open(context.Background(), c.config)
	if err != nil {
		logger.WithError(err).Fatal("failed to set up secret store plugin")
	}
	c.secretStores = append(c.secretStores, plugin)
	logger.Info("successfully set up secret store plugin")
	return c
}

func (c *Runtime) WithClientApplication(files embed.FS) *Runtime {
	c.withClientApplication(files)
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

////////////////////
// Runtime Functions
////////////////////

// Start the runtime of the service. This will do things like run the grpc server and consumers and put
// them on a background goroutine
func (c *Runtime) Start() {
	cors := c.buildCors()
	handler := cors.Handler(c.mux)
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}
	close := make(chan os.Signal, 1)
	signal.Notify(close, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	if c.isRPC {
		c.runRPC(close, handler)
	}

	go c.runMux(close, handler)

	// TODO: start consumers

	// wait for close signal
	<-close
	c.shutdown()
}

func (c *Runtime) shutdown() {
	c.logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	group := errgroup.Group{}

	// TODO: shutdown rpc server?

	// shutdown repositories
	for _, r := range c.repositories {
		r := r
		group.Go(func() error {
			e := r.Close(ctx)
			if e != nil {
				return c.logger.WithField("plugin", reflect.TypeOf(r).String()).Wrap(e)
			}
			return nil
		})
	}

	// shutdown brokers
	for _, b := range c.brokers {
		b := b
		group.Go(func() error {
			e := b.Close(false)
			if e == nil {
				return nil
			}
			c.logger.WithField("plugin", reflect.TypeOf(b).String()).Error("failed to gracefully close broker: forcing")
			e = b.Close(true)
			if e != nil {
				return c.logger.WithField("plugin", reflect.TypeOf(b).String()).Wrap(e)
			}
			return nil
		})
	}

	// wait for graceful shutdowns
	err := group.Wait()
	if err != nil {
		c.logger.WrappedError(err, "failed to shutdown gracefully")
		return
	}
	c.logger.Info("shutdown successfully")
}

// TODO -> use closer
func (c *Runtime) runRPC(close CloseChan, handler http.Handler) {
	if len(c.rpcReflectionServiceNames) > 0 {
		reflector := grpcreflect.NewStaticReflector(c.rpcReflectionServiceNames...)
		c.mux.Handle(grpcreflect.NewHandlerV1(reflector))
		c.mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}
}

// TODO -> use closer
func (c *Runtime) runMux(close CloseChan, handler http.Handler) {
	addr := fmt.Sprintf("0.0.0.0:%s", c.config.GetString("port"))
	if err := http.ListenAndServe(addr, h2c.NewHandler(handler, &http2.Server{})); err != nil {
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
