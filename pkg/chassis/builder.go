package chassis

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
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
	blueprintConn struct {
		stream *connect.BidiStreamForClient[sdv1.ClientDetails, sdv1.ClusterDetails]
		closer chan struct{}
	}
)

var closer CloseChan

func Closer() CloseChan {
	return closer
}

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

func (c *Runtime) WithClientApplication(files embed.FS, rootDir string) *Runtime {
	c.withClientApplication(files, rootDir)
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

// WithRunner adds a function to be called in a goroutine when Runtime.Start is called
func (c *Runtime) WithRunner(f func()) *Runtime {
	if c.onStart == nil {
		c.onStart = make([]func(), 0)
	}
	c.onStart = append(c.onStart, f)
	return c
}

func (c *Runtime) WithRoute(route *ntv1.Route) *Runtime {
	err := c.withRoute(route)
	if err != nil {
		c.logger.WithError(err).Panic("failed to register route")
	}
	return c
}

func (c *Runtime) DisableMux() *Runtime {
	c.noMux = true
	return c
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
	Metadata  map[string]string
}

func (c *Runtime) newBlueprintClient(useEntrypoint bool) {
	var entrypoint string
	if useEntrypoint {
		entrypoint = c.config.GetString("service.entrypoint")
	} else {
		if node := c.blueprintCluster.Pop(); node == nil {
			// TODO: determine what to do if there are not any nodes to connect to
			c.logger.Warn("no blueprint node within known cluster. falling back on service config")
			entrypoint = c.config.GetString("service.entrypoint")
		} else {
			entrypoint = node.Address
		}
	}

	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				// If you're also using this client for non-h2c traffic, you may want
				// to delegate to tls.Dial if the network isn't TCP or the addr isn't
				// in an allowlist.
				return net.Dial(network, addr)
			},
		},
	}

	c.blueprintClient = sdv1Cnt.NewServiceDiscoveryServiceClient(httpClient, entrypoint)
}

func (c *Runtime) Register(options RegistrationOptions) *Runtime {
	c.newBlueprintClient(true)
	var (
		pid *sdv1.ProcessIdentity
		err error
	)

	// TODO: this can get stuck and won't listen to sigint events if it can't initialize
	for range INTITIALIZE_LIMIT {
		// connect with `blueprint` to get an identity
		pid, err = c.initialize()
		if err != nil {
			c.logger.WithError(err).Error("failed to initialize process (may retry)")
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
	go c.synchronize(context.Background(), pid, options)

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
func (c *Runtime) synchronize(ctx context.Context, pid *sdv1.ProcessIdentity, opts RegistrationOptions) {
	var (
		conn = &blueprintConn{
			stream: c.blueprintClient.Synchronize(ctx),
			closer: make(chan struct{}),
		}
	)

	go c.receiveAck(conn)

	for {
		meta := make([]*sdv1.Metadata, 0)
		for _, v := range c.rpcServiceNames {
			meta = append(meta, &sdv1.Metadata{
				Pid:   pid.GetPid(),
				Key:   v,
				Value: v,
			})
		}

		// TODO: should we also save external host/port?
		adder := fmt.Sprintf("%s:%d", c.config.GetString("service.network.internal.host"), c.config.GetInt("service.network.internal.port"))

		req := connect.NewRequest(&sdv1.ClientDetails{
			Pid:              pid.GetPid(),
			RunningState:     sdv1.ProcessRunningState_PROCESS_RUNNING,
			HealthState:      sdv1.ProcessHealthState_PROCESS_HEALTHY,
			ProcessKind:      sdv1.ProcessKind_SERVER_PROCESS,
			Token:            pid.Token.GetJwt(),
			Location:         &sdv1.GeoPoint{},
			Metadata:         meta,
			AdvertiseAddress: adder,
		})

		err := conn.stream.Send(req.Msg)
		if err != nil {
			// If a connection is lost with the leader. Attempt to connect to other known blueprint
			// instances to find the new leader to send status to
			c.logger.WithError(err).Error("failed to send process details to blueprint (will retry)")
			c.newBlueprintClient(false)

			if c.blueprintClient == nil {
				c.logger.Error("can't connect to blueprint")
				return
			}

			conn.stream = c.blueprintClient.Synchronize(ctx)
		} else {
			c.logger.WithField("message", req.Msg).Trace("sync successful")
		}

		time.Sleep(SYNC_INTERVAL)
	}
}

// `receiveAck` processes all incoming messages from the synchronize stream. `ClusterDetails` are received and updated in a local store
// wrapped with a mutex to store any changes to a connected blueprint cluster. This lends it's self to a more realtime gossip data dissemination
// of blueprint cluster details.
func (c *Runtime) receiveAck(conn *blueprintConn) {
	for {
		in, err := conn.stream.Receive()
		if err == io.EOF {
			close(conn.closer)
			return
		}
		if err != nil {
			c.logger.WithError(err).Error("stream closed")
			close(conn.closer)
			return
		}
		c.logger.WithField("nodes", in.GetNodes()).Trace("got message")
		// when an ack message is received from the connected blueprint node is received
		// save the nodes to memory so when a failure occurs on the blueprint cluster the
		// chassis synchronize connection can be reestablished to the blueprint leader to report
		// it's status
		c.blueprintCluster.Lock()
		c.blueprintCluster.Nodes = in.GetNodes()
		c.blueprintCluster.Unlock()
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

	if c.isRPC {
		c.runRPC(handler)
	}

	if !c.noMux {
		go c.runMux(handler)
	}

	// TODO: start consumers

	if c.onStart != nil {
		for _, f := range c.onStart {
			// TODO: do we want to pass a close channel so the routine can cleanly shutdown?
			go f()
		}
	}

	// wait for close signal
	<-closer
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
func (c *Runtime) runRPC(_ http.Handler) {
	if len(c.rpcReflectionServiceNames) > 0 {
		reflector := grpcreflect.NewStaticReflector(c.rpcReflectionServiceNames...)
		c.mux.Handle(grpcreflect.NewHandlerV1(reflector))
		c.mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}
}

// TODO -> use closer
func (c *Runtime) runMux(handler http.Handler) {
	addr := fmt.Sprintf("%s:%d", c.config.GetString("service.network.bind_address"), c.config.GetInt("service.network.bind_port"))
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
