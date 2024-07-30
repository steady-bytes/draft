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

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	sdv1Cnt "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"

	"connectrpc.com/connect"
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

func (c *Runtime) WithRoute(route *ntv1.Route) *Runtime {
	err := c.withRoute(route)
	if err != nil {
		c.logger.WithError(err).Panic("failed to register route")
	}
	return c
}

func (c *Runtime) GetConfig() Config {
	return c.config
}

// /////////////////
// System Functions
// /////////////////
const (
	ErrProcessRegistrationFailed = "failed to connect to blueprint"
)

const (
	SYNC_INTERVAL     = 5 * time.Second
	INTITIALIZE_LIMIT = 5
)

type RegistrationOptions struct {
	Namespace string
}

func (c *Runtime) Register(options RegistrationOptions) *Runtime {
	entrypoint := c.config.GetString("service.entrypoint")
	c.blueprintClient = sdv1Cnt.NewServiceDiscoveryServiceClient(http.DefaultClient, entrypoint)

	var (
		pid *sdv1.ProcessIdentity
		err error
	)
	for range [INTITIALIZE_LIMIT]int{} {
		// connect with `blueprint` to get an identity
		pid, err = c.initialize()
		if err != nil {
			c.logger.WithError(err).Error("failed to initialize process")
			time.Sleep(SYNC_INTERVAL)
			continue
		}
		break
	}
	if pid == nil {
		// TODO (@andrewsc208): don't panic here, use configuration from the `RegistrationOptions` to determine error handling
		//   					In the short term this works for the current use case.
		panic(ErrProcessRegistrationFailed)
	}

	// now that a system identity has been established, we can synchronize the process state with blueprint
	// TODO: Test what would happen if the `blueprint` leader dies when synchronizing
	go c.synchronize(pid)

	return c
}

// register the `process` with `blueprint` to receive it's system identity
func (c *Runtime) initialize() (*sdv1.ProcessIdentity, error) {
	req := connect.NewRequest(&sdv1.InitializeRequest{
		Name: c.config.GetString("service.name"),
		// TODO (@andrewsc208): find a nonce generator, or use a more secure method to generate a public key for the process to use
		Nonce: "FUSE",
	})

	res, err := c.blueprintClient.Initialize(context.Background(), req)
	if err != nil {
		return nil, err
	}

	return res.Msg.GetProcessIdentity(), nil
}

// synchronize the process state with `blueprint`
// this is intended to be run in a background thread so it's non-blocking and will run indefinitely
// TODO:
//   - Test what would happen if the `blueprint` leader dies when synchronizing
//   - Add a channel to stop the synchronization
//   - Add a state machine around health, and running state of the process that can be reported by the service layer
func (c *Runtime) synchronize(pid *sdv1.ProcessIdentity) {
	req := connect.NewRequest(&sdv1.ClientDetails{
		Pid:          pid.GetPid(),
		RunningState: sdv1.ProcessRunningState_PROCESS_RUNNING,
		HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
		ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
		Token:        pid.Token.GetJwt(),
		Location:     &sdv1.GeoPoint{},
		Metadata:     []*sdv1.Metadata{},
	})

	stream := c.blueprintClient.Synchronize(context.Background())

	for {
		if err := stream.Send(req.Msg); err != nil {
			// TODO: need gracefully handle this error
			// if the leader dies then we should try to reconnect to the new leader
			c.logger.WithError(err).Fatal("failed to send process details to blueprint")
		}

		time.Sleep(SYNC_INTERVAL)
	}
}

////////////////////
// Runtime Functions
////////////////////

// Start the runtime of the service. This will do things like run the grpc server and consumers and put
// them on a background goroutine
func (c *Runtime) Start() {
	cors := c.buildCors()
	handler := h2c.NewHandler(cors.Handler(c.mux), &http2.Server{})
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
func (c *Runtime) runRPC(_ CloseChan, _ http.Handler) {
	if len(c.rpcReflectionServiceNames) > 0 {
		reflector := grpcreflect.NewStaticReflector(c.rpcReflectionServiceNames...)
		c.mux.Handle(grpcreflect.NewHandlerV1(reflector))
		c.mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}
}

// TODO -> use closer
func (c *Runtime) runMux(_ CloseChan, handler http.Handler) {
	addr := fmt.Sprintf("%s:%d", c.config.GetString("service.network.bind_address"), c.config.GetInt("service.network.port"))
	c.logger.Info(fmt.Sprintf("running server on: %s", addr))
	if err := http.ListenAndServe(addr, h2c.NewHandler(handler, &http2.Server{})); err != nil {
		c.logger.WithError(err).Panic("failed to start mux server")
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
