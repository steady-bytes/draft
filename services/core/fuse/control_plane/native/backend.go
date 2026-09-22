// Package native implements Fuse's native reverse-proxy ProxyBackend --
// Fuse terminating connections itself instead of configuring a separate
// Envoy process. See
// docs/website/content/docs/architecture/fuse-native-proxy.md and its
// implementation plan for the design this package builds toward. This
// file covers Phase 2 (route table, HTTP/1.1 + h2c reverse proxying,
// endpoint selection), wires in Phase 3's auth middleware (auth.go),
// Phase 4's WideEvent production (wide_event.go), and Phase 5's TLS
// termination (tls.go, tls_selfsigned.go, tls_static.go). mTLS (Phase 6)
// is not implemented yet.
package native

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	cp "github.com/steady-bytes/draft/services/core/fuse/control_plane"

	"golang.org/x/net/http2"
)

// RouteTable is an immutable snapshot of the routes Apply was last called
// with -- already merged by controlPlane (one entry per logical route
// name). Swapped atomically on every Apply so a request in flight always
// sees a consistent table, never a partially-updated one.
type RouteTable struct {
	routes []*ntv1.Route
}

// Backend is the native ProxyBackend: Fuse terminates connections itself
// with a stdlib net/http server instead of configuring a separate Envoy
// process.
type Backend struct {
	logger       chassis.Logger
	table        atomic.Pointer[RouteTable]
	server       *http.Server
	authMW       *authMiddleware
	certProvider CertificateProvider // nil when fuse.tls.mode is "off" (the default)

	// roundRobin is a single shared counter used to spread requests across
	// a multi-endpoint route's backends -- see pickEndpoint.
	roundRobin atomic.Uint64
}

// NewBackend constructs the native backend and its listener, but does not
// start accepting connections -- see ListenAndServe, invoked from a
// chassis.WithRunner goroutine by main.go. Panics on an unrecoverable TLS
// setup failure (bad operator cert/key, CA generation/persistence
// failure), matching NewEnvoyBackend's own fail-fast-at-startup style
// rather than threading an error return through main.go's construction
// chain.
func NewBackend(logger chassis.Logger) *Backend {
	b := &Backend{logger: logger, authMW: newAuthMiddleware()}
	b.table.Store(&RouteTable{})

	tlsConfig := b.buildTLSConfig(logger)

	b.server = &http.Server{
		Addr: listenerAddr(),
		// h2c.NewHandler lets this one listener accept HTTP/1.1 and
		// cleartext HTTP/2 on the same port -- Envoy's HttpConnectionManager
		// already does both without extra configuration; net/http needs the
		// explicit wrapper for the h2c half. Harmless to leave wrapped even
		// when TLS is active: ALPN negotiation happens before this handler
		// ever sees the request, and h2c.NewHandler only intercepts the
		// specific cleartext-HTTP/2 preface, passing everything else
		// through unchanged.
		Handler:   h2cHandler(http.HandlerFunc(b.serveHTTP)),
		TLSConfig: tlsConfig,
	}
	return b
}

// buildTLSConfig reads fuse.tls.mode and constructs the matching
// CertificateProvider, returning nil when TLS is off (today's default --
// a cluster that never sets this key sees no behavior change). Stores the
// provider on b for Phase 6's mTLS to use later.
func (b *Backend) buildTLSConfig(logger chassis.Logger) *tls.Config {
	config := chassis.GetConfig()
	mode := config.GetString(TLSModeConfigKey)
	if mode == "" {
		mode = TLSModeOff
	}

	var provider CertificateProvider
	var err error
	switch mode {
	case TLSModeOff:
		return nil
	case TLSModeSelfSigned:
		provider, err = NewSelfSignedProvider(logger)
	case TLSModeOperator:
		provider, err = NewStaticProvider(
			config.GetString("fuse.tls.operator.cert_file"),
			config.GetString("fuse.tls.operator.key_file"),
			config.GetString("fuse.tls.operator.client_ca_file"),
		)
	case TLSModeACME:
		directoryURL := config.GetString("fuse.tls.acme.directory_url")
		if directoryURL == "" {
			directoryURL = acmeDefaultDirectoryURL
		}
		provider = NewACMEProvider(directoryURL, config.GetString("fuse.tls.acme.cache_dir"), &b.table)
	default:
		logger.WithField("mode", mode).Panic("unknown fuse.tls.mode")
		return nil
	}
	if err != nil {
		logger.WithError(err).Panic("failed to initialize TLS certificate provider")
		return nil
	}

	b.certProvider = provider
	go b.logCertificateChanges(provider)

	return tlsConfigForProvider(provider, &b.table)
}

// tlsConfigForProvider builds the per-handshake SNI-based certificate
// selection every TLS mode shares, regardless of where the provider's
// certificates come from, plus mTLS enforcement (Phase 6) for any route
// sharing a host with an mtls.enabled route. A standalone function (not
// inlined into buildTLSConfig) so tests can exercise the exact same wiring
// a real listener uses without needing a full Backend/Blueprint. table may
// be nil (no mTLS lookups performed -- every handshake gets a plain server
// cert), which is what the Phase 5 handshake tests still pass.
func tlsConfigForProvider(provider CertificateProvider, table *atomic.Pointer[RouteTable]) *tls.Config {
	return &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			cert, err := provider.GetCertificate(hello.ServerName)
			if err != nil {
				return nil, err
			}
			cfg := &tls.Config{
				Certificates: []tls.Certificate{*cert},
				NextProtos:   []string{"h2", "http/1.1"},
			}

			if table == nil {
				return cfg, nil
			}
			route, ok := routeForMTLS(table.Load(), hello.ServerName)
			if !ok {
				return cfg, nil
			}
			pool, err := provider.GetCACertPool(route.GetMtls().GetTrustedCaSecretName())
			if err != nil {
				// Deny the connection rather than silently skip mTLS on a
				// config error -- serving without the client-cert
				// requirement this route asked for would be a silent
				// security downgrade, not a graceful degradation.
				return nil, err
			}
			cfg.ClientAuth = tls.RequireAndVerifyClientCert
			cfg.ClientCAs = pool
			return cfg, nil
		},
	}
}

// logCertificateChanges is the "purely for logging/metrics in this phase"
// consumer of CertificateProvider.Changed() the design doc calls for -- no
// listener restart is needed on a change, since GetConfigForClient always
// calls GetCertificate fresh on the next handshake regardless.
func (b *Backend) logCertificateChanges(provider CertificateProvider) {
	for domain := range provider.Changed() {
		b.logger.WithField("domain", domain).Info("native backend: certificate renewed")
	}
}

// listenerAddr reads the same fuse.listener.address/port config keys the
// envoy backend uses to build its own listener resource -- shared config
// surface, see control_plane/backend.go. main.go's validatePortSeparation
// already guarantees this differs from Fuse's own control-plane RPC port.
func listenerAddr() string {
	config := chassis.GetConfig()
	address := config.GetString(cp.LISTENER_ADDRESS_CONFIG_KEY)
	if address == "" {
		address = cp.LISTENER_DEFAULT_ADDRESS
	}
	port := config.GetUint32(cp.LISTENER_PORT_CONFIG_KEY)
	if port == 0 {
		port = cp.LISTENER_DEFAULT_PORT
	}
	return fmt.Sprintf("%s:%d", address, port)
}

// ListenAndServe blocks, serving proxied traffic until the server is
// closed or fails. Intended to run in its own goroutine (chassis's
// WithRunner already provides one) -- callers should treat
// http.ErrServerClosed as a clean shutdown, not a failure.
func (b *Backend) ListenAndServe() error {
	b.logger.WithField("address", b.server.Addr).WithField("tls", b.server.TLSConfig != nil).Info("native proxy backend listening")
	if b.server.TLSConfig != nil {
		// Empty cert/key file arguments are valid here: TLSConfig's own
		// GetConfigForClient (set in buildTLSConfig) supplies certificates
		// per-handshake -- net/http's own docs describe this as the
		// supported way to rely on GetCertificate/GetConfigForClient
		// instead of static files.
		return b.server.ListenAndServeTLS("", "")
	}
	return b.server.ListenAndServe()
}

func (b *Backend) Name() string { return cp.ProxyBackendNative }

// Capabilities: WideEvents (Phase 4) and TLS (Phase 5, tls.go) are true.
// MTLS/ACME stay false until Phases 6-7 land. This reports what the code
// supports, not current config -- true here regardless of whether
// fuse.tls.mode is actually "off" right now, the same way
// EnvoyBackend.Capabilities() reports false because that code path is
// unbuilt, not because of any config value. All four are now true: TLS
// (Phase 5), MTLS (Phase 6), WideEvents (Phase 4), ACME (Phase 7).
func (b *Backend) Capabilities() cp.BackendCapabilities {
	return cp.BackendCapabilities{TLS: true, MTLS: true, WideEvents: true, ACME: true}
}

// Apply swaps in a new route table. No error path: unlike the envoy
// backend (which can fail marshaling an Envoy snapshot), storing a Go
// slice behind an atomic.Pointer cannot fail.
func (b *Backend) Apply(routes []*ntv1.Route) error {
	b.table.Store(&RouteTable{routes: routes})
	return nil
}

func (b *Backend) serveHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := matchRoute(b.table.Load(), stripPort(r.Host), r.URL.Path)
	if !ok {
		http.NotFound(w, r) // deny-by-default, same as Envoy's "no matching virtual host"
		return
	}

	if authRequired(route) {
		authAddr := authAddress(r.Context())
		// Empty address means no auth service is registered -- treated as
		// public, the same convention envoy_backend.go's Apply uses
		// (authEnabled := authAddr != "").
		if authAddr != "" && !b.authMW.check(w, r, authAddr) {
			return // check already wrote the denial response
		}
	}

	target, err := b.pickEndpoint(route)
	if err != nil {
		b.logger.WithField("route_name", route.GetName()).WithError(err).Error("route has no endpoints")
		http.Error(w, "no upstream available", http.StatusBadGateway)
		return
	}

	// WideEvent production (see docs/architecture/wide-events.md): gated on
	// the same global flag every other WideEvent producer in the framework
	// uses, plus this route's own opt-out. telemetry.enabled (not checked
	// here) is a second, independent gate inside chassis.Span.End itself --
	// see wide_event.go's package comment.
	var span *chassis.Span
	emit := chassis.GetConfig().GetBool("telemetry.wide_events.enabled") && !route.GetWideEventsDisabled()
	var rw http.ResponseWriter = w
	var rec *statusRecorder
	var upstreamStart time.Time
	if emit {
		// Continue the caller's trace (eg. a bench workflow step's
		// traceparent header) instead of always starting a new, disconnected
		// one -- see ContinueTraceFromHeader's doc comment. A request with no
		// traceparent header (or a malformed one) leaves ctx unchanged, so
		// StartSpan below begins a fresh trace exactly as before.
		ctx := chassis.ContinueTraceFromHeader(r.Context(), r.Header)
		ctx, span = chassis.StartSpan(ctx, route.GetName())
		r = r.WithContext(ctx)
		rec = &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		rw = rec
		upstreamStart = time.Now()
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: target})
			// Envoy's default router never rewrites the Host/authority header
			// (no HostRewriteSpecifier is set anywhere in envoy_backend.go) --
			// SetURL alone would overwrite it to the target's host, which is
			// not what Envoy does today and would break subdomain-per-service
			// routing on the upstream side. Preserve the original inbound
			// Host explicitly to match.
			pr.Out.Host = pr.In.Host
			// Propagate this hop's own span onward so the upstream's own
			// tracing (if any) continues the same trace Fuse just joined,
			// instead of the chain stopping at the proxy. Only set when a
			// span exists (WideEvents enabled for this route) -- see emit
			// above.
			if tp, ok := chassis.TraceParentHeader(pr.Out.Context()); ok {
				pr.Out.Header.Set("traceparent", tp)
			}
		},
	}
	if route.GetEnableHttp2() {
		proxy.Transport = h2cUpstreamTransport
	}
	proxy.ServeHTTP(rw, r)

	if span != nil {
		span.SetAttribute("http.method", r.Method)
		span.SetAttribute("http.path", r.URL.Path)
		span.SetAttribute("http.status_code", strconv.Itoa(rec.status))
		span.SetAttribute("route.name", route.GetName())
		span.SetAttribute("route.match_type", route.GetMatch().GetMatchType().String())
		span.SetRuntimeAttribute("upstream.address", target)
		span.SetRuntimeAttribute("upstream.duration_ms", strconv.FormatInt(time.Since(upstreamStart).Milliseconds(), 10))
		if r.TLS != nil {
			span.SetRuntimeAttribute("tls.version", tls.VersionName(r.TLS.Version))
			span.SetRuntimeAttribute("tls.cipher_suite", tls.CipherSuiteName(r.TLS.CipherSuite))
			// Only set on mTLS routes -- PeerCertificates is empty unless
			// ClientAuth required and verified one (see routeForMTLS).
			if len(r.TLS.PeerCertificates) > 0 {
				span.SetRuntimeAttribute("tls.client_cert_subject", r.TLS.PeerCertificates[0].Subject.String())
			}
		}
		span.End(nil)
	}
}

// pickEndpoint round-robins across a route's registered endpoints, the
// same load-balancing makeCluster's ROUND_ROBIN LbPolicy gives the envoy
// backend for a multi-endpoint route. One shared counter across every
// route (rather than one per route) is a deliberate simplification -- the
// modulo is taken against each route's own endpoint count, so a request
// still spreads evenly across that route's backends, and this avoids
// needing per-route state that would otherwise have to survive across
// Apply's route table swaps.
func (b *Backend) pickEndpoint(r *ntv1.Route) (string, error) {
	endpoints := r.GetEndpoints()
	if len(endpoints) == 0 {
		if ep := r.GetEndpoint(); ep != nil {
			endpoints = []*ntv1.Endpoint{ep}
		}
	}
	if len(endpoints) == 0 {
		return "", fmt.Errorf("route %q has no endpoints", r.GetName())
	}
	idx := b.roundRobin.Add(1) % uint64(len(endpoints))
	ep := endpoints[idx]
	return fmt.Sprintf("%s:%d", ep.GetHost(), ep.GetPort()), nil
}

// h2cUpstreamTransport proxies enable_http2 routes to the upstream in
// cleartext HTTP/2 -- matches makeCluster's Http2ProtocolOptions opt-in
// (there is no TLS between Fuse and the upstream cluster either way,
// before or after this change).
var h2cUpstreamTransport = &http2.Transport{
	AllowHTTP: true,
	DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	},
}

// stripPort mirrors the envoy backend's StripAnyHostPort HCM setting
// (envoy_backend.go) -- a browser includes a non-default port in its Host
// header, but routes are registered with a bare host.
func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
