package main

import (
	"fmt"
	"net/http"

	"github.com/steady-bytes/draft/pkg/chassis"

	cp "github.com/steady-bytes/draft/services/core/fuse/control_plane"
	"github.com/steady-bytes/draft/services/core/fuse/control_plane/native"
)

func main() {
	chassis.NewMetricsReporter().Start()

	validatePortSeparation()

	var (
		logger  = chassis.NewOTelLogger()
		backend = newBackend(logger)
		// fuse control plane: route validation, conflict detection, and
		// Blueprint-KV persistence -- identical regardless of which
		// ProxyBackend is active.
		controlPlane = cp.NewControlPlane(logger, backend)
		// fuse control plane rpc interface
		controlPlaneRPC = cp.NewRPC(logger, controlPlane)
	)

	runtime := chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "core",
		}).
		WithRPCHandler(controlPlaneRPC).
		// make sure to load the cache on boot
		WithRunner(controlPlane.LoadCache)

	switch b := backend.(type) {
	case *cp.EnvoyBackend:
		// The xDS/ADS surface only makes sense when a separate Envoy
		// process is actually consuming it -- registering it
		// unconditionally would open a gRPC surface that does nothing when
		// the native backend is active.
		runtime = runtime.WithRPCHandler(cp.NewXDSRpc(logger, b))
	case *native.Backend:
		// The native backend is its own listener, not an RPC handler
		// chassis's Rpcer wraps -- it needs its own long-running goroutine,
		// the same way LoadCache gets one above.
		runtime = runtime.WithRunner(func() {
			if err := b.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.WithError(err).Panic("native proxy backend failed")
			}
		})
	}

	defer runtime.Start()
}

// newBackend constructs the ProxyBackend selected by fuse.proxy_backend
// (see cp.ProxyBackendName). This is the one place a new backend gets added
// -- the same explicit-construction shape chassis.Runtime.WithBroker/
// WithRepository already use for their own plugins, not a string-keyed
// registry.
func newBackend(logger chassis.Logger) cp.ProxyBackend {
	switch cp.ProxyBackendName() {
	case cp.ProxyBackendNative:
		return native.NewBackend(logger)
	default:
		return cp.NewEnvoyBackend(logger)
	}
}

// validatePortSeparation guards against Fuse's control-plane RPC server
// (service.network.bind_port) and its data-plane proxy listener
// (fuse.listener.port) being configured to the same port. On the envoy
// backend a collision here was merely confusing -- two independent
// processes, each free to bind the same port number until one actually
// loses the race. On the native backend (once it exists) it would be a
// guaranteed same-process double-bind failure moments later regardless, so
// this fails fast with a named cause instead of leaving it to surface as a
// generic "address already in use" from the OS.
func validatePortSeparation() {
	var (
		config      = chassis.GetConfig()
		controlPort = config.GetUint32("service.network.bind_port")
		proxyPort   = config.GetUint32(cp.LISTENER_PORT_CONFIG_KEY)
	)
	if proxyPort == 0 {
		proxyPort = cp.LISTENER_DEFAULT_PORT
	}
	if controlPort == proxyPort {
		panic(fmt.Sprintf(
			"fuse.listener.port (%d) must differ from service.network.bind_port (%d) -- "+
				"Fuse cannot bind the same port for both its control-plane RPC server and its data-plane proxy listener",
			proxyPort, controlPort,
		))
	}
}
