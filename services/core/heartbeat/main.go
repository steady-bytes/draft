// Command heartbeat is a minimal core service that exists to exercise chassis's
// effect-tracking primitive (chassis.Effect, see pkg/chassis/effect.go) end to end,
// against a real Blueprint cluster instead of just chassis's own unit tests.
//
// It demonstrates both paths Effect supports:
//   - WithRepository, a built-in plugin kind now implemented on top of Effect
//     (repository.go's memoryRepository — Open on startup, Close on shutdown).
//   - A direct, ad hoc c.Effect call for a KV write to Blueprint that needs a real
//     inverse — the exact shape that was missing from services/core/auth's
//     writeAuthAddress, done correctly from the start here.
//
// Registration order matters: the repository is registered first, then the KV
// effect. Under LIFO teardown, that means shutdown retracts the Blueprint KV key
// before closing the repository — watch the log order on SIGINT/SIGTERM to see it.
//
// See docs/website/content/docs/architecture/chassis-composability.md for the full
// design this service is a live test of.
package main

import (
	"context"
	"fmt"
	"net/http"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/anypb"
)

// heartbeatBlueprintKey is the Blueprint KV key this service publishes its address
// under while it's alive.
const heartbeatBlueprintKey = "heartbeat_service_address"

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	// chassis.New(logger) calls logger.Start(config), which is what gives the
	// zerolog wrapper its real output writer. A child logger derived via
	// WithField/WithFields *before* that point binds to zerolog's uninitialized
	// zero-value writer and silently discards everything logged through it — so
	// repo's own logger has to be derived after chassis.New, not before.
	c := chassis.New(logger)

	repo := newMemoryRepository(logger)
	c.WithRepository(repo).
		Register(chassis.RegistrationOptions{Namespace: "core"})

	c.Effect(heartbeatBlueprintKey, func() (func(context.Context) error, error) {
		if err := writeHeartbeatAddress(logger); err != nil {
			return nil, err
		}
		return func(ctx context.Context) error {
			return deleteHeartbeatAddress(ctx, logger)
		}, nil
	})

	defer c.Start()
}

func writeHeartbeatAddress(logger chassis.Logger) error {
	config := chassis.GetConfig()
	addr := fmt.Sprintf("http://%s:%d",
		config.GetString("service.network.internal.host"),
		config.GetInt("service.network.internal.port"))

	val, err := anypb.New(&kvv1.Value{Data: addr})
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", heartbeatBlueprintKey, err)
	}

	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, config.Entrypoint())
	if _, err := client.Set(context.Background(), connect.NewRequest(&kvv1.SetRequest{
		Key: heartbeatBlueprintKey, Value: val,
	})); err != nil {
		return fmt.Errorf("failed to write %s to blueprint: %w", heartbeatBlueprintKey, err)
	}
	logger.WithField("address", addr).Info("registered heartbeat address with blueprint")
	return nil
}

func deleteHeartbeatAddress(ctx context.Context, logger chassis.Logger) error {
	config := chassis.GetConfig()
	val, err := anypb.New(&kvv1.Value{}) // DeleteRequest.Value only carries type info
	if err != nil {
		return fmt.Errorf("failed to build delete request for %s: %w", heartbeatBlueprintKey, err)
	}

	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, config.Entrypoint())
	if _, err := client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{
		Key: heartbeatBlueprintKey, Value: val,
	})); err != nil {
		return fmt.Errorf("failed to delete %s from blueprint: %w", heartbeatBlueprintKey, err)
	}
	logger.Info("retracted heartbeat address from blueprint")
	return nil
}
