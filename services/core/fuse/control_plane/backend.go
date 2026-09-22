package control_plane

import (
	"context"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/anypb"
)

// ProxyBackend abstracts what happens to a validated route table once
// controlPlane has persisted it to Blueprint's key/value store. Route
// validation, conflict detection, and persistence (controlPlane, in
// controller.go) are identical regardless of which backend is active --
// Apply is the only backend-specific step. See
// docs/website/content/docs/architecture/fuse-native-proxy.md for the
// design this implements.
type ProxyBackend interface {
	// Apply is called with the full current, merged route table (one entry
	// per logical route name -- see mergeRoutes) whenever it changes.
	Apply(routes []*ntv1.Route) error

	// Capabilities reports what this backend can actually enforce, so a
	// route/TLS/mTLS/WideEvent setting the active backend can't honor fails
	// clearly at config time instead of being silently accepted and
	// ignored. See rpc.go's AddRoute/ValidateRoute.
	Capabilities() BackendCapabilities

	Name() string
}

// BackendCapabilities reports feature support for the active ProxyBackend.
// Kept as a flat set of booleans, not a versioned or extensible scheme,
// until a real second axis is needed.
type BackendCapabilities struct {
	TLS        bool
	MTLS       bool
	WideEvents bool
	ACME       bool
}

const (
	// ProxyBackendConfigKey selects which ProxyBackend main.go constructs
	// at startup.
	ProxyBackendConfigKey = "fuse.proxy_backend"

	ProxyBackendEnvoy  = "envoy"
	ProxyBackendNative = "native"

	// proxyBackendDefault is temporarily ProxyBackendEnvoy: there is no
	// native backend to fall back to yet. The implementation plan's Phase 3
	// flips this to ProxyBackendNative once the native backend reaches
	// auth parity with the envoy backend -- see
	// docs/website/content/docs/architecture/fuse-native-proxy-implementation-plan.md#34-flip-the-default-to-native.
	// Do not read this value as a decision that envoy should stay the
	// long-term default; it is not.
	proxyBackendDefault = ProxyBackendEnvoy

	// LISTENER_ADDRESS_CONFIG_KEY / LISTENER_PORT_CONFIG_KEY name the
	// data-plane listener's bind address/port -- shared config surface
	// both backends read: the envoy backend uses it to build the Envoy
	// listener resource pushed over xDS; the native backend will bind it
	// directly once it exists. This must differ from
	// "service.network.bind_port" (Fuse's own control-plane RPC port) --
	// see main.go's validatePortSeparation.
	LISTENER_ADDRESS_CONFIG_KEY = "fuse.listener.address"
	LISTENER_PORT_CONFIG_KEY    = "fuse.listener.port"
	LISTENER_DEFAULT_ADDRESS    = "0.0.0.0"
	LISTENER_DEFAULT_PORT       = 80
)

// ProxyBackendName resolves fuse.proxy_backend, falling back to
// proxyBackendDefault when unset.
func ProxyBackendName() string {
	if v := chassis.GetConfig().GetString(ProxyBackendConfigKey); v != "" {
		return v
	}
	return proxyBackendDefault
}

// GetAuthServiceAddress reads the auth service's discovered address from
// Blueprint KV (written by services/core/auth's writeAuthAddress on boot).
// Returns "" if no auth service is registered -- every backend treats an
// empty address as "no auth service; every route is public," the same
// convention envoy_backend.go's Apply already used before this was shared.
// Exported and package-level (not a method on either backend) so both
// EnvoyBackend and the native backend call the identical lookup -- this is
// the "candidate to hoist into a shared package" the design doc's matcher
// section flagged, applied here instead since this lookup is simpler than
// the matcher and had no reason to wait.
func GetAuthServiceAddress(ctx context.Context, client kvv1Connect.KeyValueServiceClient) string {
	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		return ""
	}
	resp, err := client.Get(ctx, connect.NewRequest(&kvv1.GetRequest{
		Key:   AuthServiceBlueprintKey,
		Value: val,
	}))
	if err != nil {
		return ""
	}
	value := &kvv1.Value{}
	if err := resp.Msg.GetValue().UnmarshalTo(value); err != nil {
		return ""
	}
	return value.Data
}
