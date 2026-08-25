package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	userv1 "github.com/steady-bytes/draft/api/examples/user/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
)

const serviceName = "examples-consumer"

// eventTypes is the exhaustive list of event types this service processes.
// One Consume stream is opened per type so that each (consumer, event_type) pair
// appears as a distinct edge in the topology view once Phase 4 registration
// tracking is deployed. The broker broadcasts all events to all streams; each
// goroutine filters to its declared type so events are never double-processed.
var eventTypes = []string{
	"examples.crud.v1.DatabaseModelSaved",
	"examples.user.v1.UserCreated",
	"examples.user.v1.UserLoggedIn",
}

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

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

	go func() {
		<-chassis.Closer()
		cancel()
	}()

	client := acConnect.NewConsumerClient(h2cClient(), catalystAddr, connect.WithGRPC())

	done := make(chan struct{}, len(eventTypes))
	for _, et := range eventTypes {
		et := et
		go func() {
			defer func() { done <- struct{}{} }()
			consumeType(ctx, logger, client, et)
		}()
	}

	for range eventTypes {
		<-done
	}
}

func consumeType(ctx context.Context, logger chassis.Logger, client acConnect.ConsumerClient, eventType string) {
	req := connect.NewRequest(&acv1.ConsumeRequest{
		Message: &acv1.CloudEvent{
			Source: serviceName,
			Type:   eventType,
		},
	})

	stream, err := client.Consume(ctx, req)
	if err != nil {
		logger.WithField("error", err.Error()).WithField("type", eventType).Error("failed to open consume stream")
		return
	}

	logger.WithField("type", eventType).Info("consumer stream open")

	for stream.Receive() {
		event := stream.Msg().GetMessage()
		if event == nil || event.Type != eventType {
			continue
		}
		handle(logger, event)
	}

	if err := stream.Err(); err != nil && ctx.Err() == nil {
		logger.WithField("error", err.Error()).WithField("type", eventType).Error("consume stream ended with error")
	}
}

func handle(logger chassis.Logger, event *acv1.CloudEvent) {
	body := []byte(event.GetTextData())

	switch event.Type {
	case "examples.crud.v1.DatabaseModelSaved":
		var payload crudv1.DatabaseModelSaved
		if err := protojson.Unmarshal(body, &payload); err != nil {
			logger.WithField("error", err.Error()).Error("failed to unmarshal DatabaseModelSaved")
			return
		}
		// logger.
		// 	WithField("model_id", payload.ModelId).
		// 	WithField("model_name", payload.ModelName).
		// 	WithField("operation", payload.Operation.String()).
		// 	Info("DatabaseModelSaved received")

	case "examples.user.v1.UserCreated":
		var payload userv1.UserCreated
		if err := protojson.Unmarshal(body, &payload); err != nil {
			logger.WithField("error", err.Error()).Error("failed to unmarshal UserCreated")
			return
		}
		// logger.
		// 	WithField("user_id", payload.UserId).
		// 	WithField("email", payload.Email).
		// 	WithField("name", payload.Name).
		// 	Info("UserCreated received")

	case "examples.user.v1.UserLoggedIn":
		var payload userv1.UserLoggedIn
		if err := protojson.Unmarshal(body, &payload); err != nil {
			logger.WithField("error", err.Error()).Error("failed to unmarshal UserLoggedIn")
			return
		}
		// logger.
		// 	WithField("user_id", payload.UserId).
		// 	WithField("email", payload.Email).
		// 	WithField("login_at", payload.LoginAt.AsTime().String()).
		// 	Info("UserLoggedIn received")

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
