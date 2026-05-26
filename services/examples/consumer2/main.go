package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	echov1 "github.com/steady-bytes/draft/api/examples/echo/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
)

const serviceName = "examples-consumer2"

var eventTypes = []string{
	"examples.echo.v1.HelloWorld",
	"examples.echo.v1.Ping",
	"examples.echo.v1.Pong",
}

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

	go func() {
		<-chassis.Closer()
		cancel()
	}()

	client := acConnect.NewConsumerClient(h2cClient(), catalystAddr, connect.WithGRPC())

	done := make(chan struct{}, len(eventTypes))
	for _, et := range eventTypes {
		et := et
		// Create a goroutine-local logger here (sequential) so that the goroutine
		// never shares a logger instance with another goroutine. Concurrent
		// WithField chains on a shared logger race on the zerolog context byte
		// slice's underlying array in Go 1.24+.
		etLogger := logger.WithField("event_type", et)
		go func() {
			defer func() { done <- struct{}{} }()
			consumeType(ctx, etLogger, client, et)
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
	case "examples.echo.v1.HelloWorld":
		var payload echov1.HelloWorld
		if err := protojson.Unmarshal(body, &payload); err != nil {
			logger.WithField("error", err.Error()).Error("failed to unmarshal HelloWorld")
		}

	case "examples.echo.v1.Ping":
		var payload echov1.Ping
		if err := protojson.Unmarshal(body, &payload); err != nil {
			logger.WithField("error", err.Error()).Error("failed to unmarshal Ping")
		}

	case "examples.echo.v1.Pong":
		var payload echov1.Pong
		if err := protojson.Unmarshal(body, &payload); err != nil {
			logger.WithField("error", err.Error()).Error("failed to unmarshal Pong")
		}
	}
}

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
