package main

import (
	"context"
	"fmt"
	"net/http"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/types/known/anypb"
)

// authServiceBlueprintKey is the Blueprint KV key Fuse reads to discover this service.
// Must match AuthServiceBlueprintKey in services/core/fuse/control_plane/controller.go.
const authServiceBlueprintKey = "auth_service_address"

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()
	cfg := loadAuthConfig()

	handler := newCheckHandler(cfg, logger)

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "core",
		}).
		// This service runs its own bare HTTP server on service.network.bind_port
		// (serveCheckEndpoint) instead of registering handlers through chassis's
		// RPC mux -- DisableMux is required here, or chassis's own Runtime.Start
		// also binds that same port with an empty *http.ServeMux (since nothing
		// ever calls WithRPCHandler/AddHandler), and the two race for the
		// listener. Found live: with both racing, chassis's empty mux won every
		// time in this environment, meaning Envoy's ext_authz check request never
		// reached checkHandler at all -- it hit an unhandled empty ServeMux, whose
		// findHandler/matchOrRedirect panics on Go's 1.22+ mux when zero patterns
		// are registered, closing the connection and making Envoy deny the
		// request via FailureModeAllow: false.
		DisableMux().
		WithRunner(func() {
			writeAuthAddress(logger)
			serveCheckEndpoint(handler, logger)
		}).
		Start()
}

// writeAuthAddress publishes this service's address to Blueprint KV so Fuse can
// discover it and enable the ext_authz filter on the Envoy listener.
func writeAuthAddress(logger chassis.Logger) {
	config := chassis.GetConfig()
	host := config.GetString("service.network.internal.host")
	port := config.GetInt("service.network.internal.port")
	addr := fmt.Sprintf("http://%s:%d", host, port)

	val, err := anypb.New(&kvv1.Value{Data: addr})
	if err != nil {
		logger.WithError(err).Error("failed to marshal auth_service_address")
		return
	}

	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, config.Entrypoint())
	_, err = client.Set(context.Background(), connect.NewRequest(&kvv1.SetRequest{
		Key:   authServiceBlueprintKey,
		Value: val,
	}))
	if err != nil {
		logger.WithError(err).Error("failed to write auth_service_address to blueprint")
		return
	}
	logger.WithField("address", addr).Info("registered auth_service_address with blueprint")
}

// serveCheckEndpoint starts the HTTP server that handles Envoy ext_authz check requests.
func serveCheckEndpoint(handler http.Handler, logger chassis.Logger) {
	config := chassis.GetConfig()
	addr := fmt.Sprintf("%s:%d",
		config.GetString("service.network.bind_address"),
		config.GetInt("service.network.bind_port"),
	)
	logger.WithField("addr", addr).Info("auth check server listening")
	if err := http.ListenAndServe(addr, h2c.NewHandler(handler, &http2.Server{})); err != nil {
		logger.WithError(err).Error("auth check server stopped")
	}
}
