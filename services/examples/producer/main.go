package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
	"github.com/steady-bytes/draft/services/examples/producer/events"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

func main() {
	logger := zerolog.New()

	defer chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "examples",
		}).
		WithRunner(func() {
			run(logger)
		}).
		Start()
}

func run(logger chassis.Logger) {
	cfg := chassis.GetConfig()
	catalystAddr := cfg.GetString("catalyst.address")
	if catalystAddr == "" {
		catalystAddr = "http://localhost:2220"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// cancel when the chassis signals shutdown
	go func() {
		<-chassis.Closer()
		cancel()
	}()

	client := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	stream := client.Produce(ctx)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// fixed IDs so repeated ticks are easy to follow in the store view
	modelID := uuid.NewString()
	userID := uuid.NewString()

	ops := []crudv1.Operation{
		crudv1.Operation_OPERATION_CREATE,
		crudv1.Operation_OPERATION_UPDATE,
		crudv1.Operation_OPERATION_DELETE,
	}
	opIdx := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			op := ops[opIdx%len(ops)]
			opIdx++
			emit(logger, stream, func() (*acv1.CloudEvent, error) {
				return events.NewDatabaseModelSaved(modelID, "User", op)
			})
			emit(logger, stream, func() (*acv1.CloudEvent, error) {
				return events.NewUserCreated(userID, "alice@example.com", "Alice")
			})
			emit(logger, stream, func() (*acv1.CloudEvent, error) {
				return events.NewUserLoggedIn(userID, "alice@example.com")
			})
		}
	}
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), which
// is required for gRPC streaming against catalyst's plain-TCP server.
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

func emit(logger chassis.Logger, stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse], build func() (*acv1.CloudEvent, error)) {
	event, err := build()
	if err != nil {
		logger.WithField("error", err.Error()).Error("failed to build event")
		return
	}
	if err := stream.Send(&acv1.ProduceRequest{Message: event}); err != nil {
		logger.WithField("error", err.Error()).Error("failed to send event")
		return
	}
	logger.WithField("type", event.Type).WithField("id", event.Id).Info("event produced")
}
