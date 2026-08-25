package main

import (
	"context"
	"crypto/tls"
	"math/rand/v2"
	"net"
	"net/http"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/examples/producer/events"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

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

	client := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	stream := client.Produce(ctx)

	for {
		delay, count := nextBurst()

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			for range count {
				emit(logger, stream, events.Random)
			}
		}
	}
}

// nextBurst returns a randomised (delay, eventCount) pair that models three
// traffic modes observed in real applications:
//
//   - Normal  (80 %): 100 ms – 1 s delay,   50 – 150 events    — steady background activity
//   - Quiet   (15 %): 2 s – 8 s delay,       2 – 10 events     — idle gaps between actions
//   - Burst    (5 %): 10 ms – 150 ms delay, 800 – 1 200 events — spikes from batch jobs / fan-out
func nextBurst() (time.Duration, int) {
	n := rand.IntN(20)
	switch {
	case n < 1: // 5 %: burst
		return randDuration(10, 150), rand.IntN(1500) + 800
	case n < 4: // 15 %: quiet
		return randDuration(2000, 8000), rand.IntN(100) + 2
	default: // 80 %: normal
		return randDuration(100, 1000), rand.IntN(1000) + 50
	}
}

func randDuration(minMs, maxMs int) time.Duration {
	return time.Duration(rand.IntN(maxMs-minMs)+minMs) * time.Millisecond
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
