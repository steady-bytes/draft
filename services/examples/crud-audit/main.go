// crud-audit is a standing Catalyst consumer for examples.crud.v1.ModelEvent (see
// services/examples/crud-event/service/events.go) — it exists for two reasons: to make that event
// type show up as a real, always-on edge in Catalyst's topology view (GetTopology only draws an
// edge for a type with an actual registered consumer — see
// services/core/catalyst/broker/controller.go), and to give every crud-event operation an audit
// trail: a WideEvent recording who did what to which model and when, queryable in Beacon the same
// way any other service's WideEvents are (service_name = "crud-audit").
//
// Unlike services/examples/consumer (a one-shot demo that opens its Consume stream once and never
// retries), this reconnects for the lifetime of the process on any stream failure — the same
// discipline services/core/blueprint/key_value/type_mutation_consumer.go uses for its own standing
// subscription, and for the same reason: an auditor that silently stops listening after the first
// Catalyst restart isn't one.
package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	serviceName    = "/services/examples/crud-audit"
	modelEventType = "examples.crud.v1.ModelEvent"
	// reconnectDelay matches type_mutation_consumer.go's own constant/reasoning: Blueprint (and
	// so Catalyst's own place in cluster startup order) means the very first Consume attempt in
	// a freshly started cluster can legitimately fail before Catalyst is up yet.
	reconnectDelay = 2 * time.Second
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

	client := acConnect.NewConsumerClient(h2cClient(), catalystAddr, connect.WithGRPC())

	for {
		if err := consume(ctx, logger, client); err != nil && ctx.Err() == nil {
			logger.WithField("error", err.Error()).Warn("crud-audit consume stream ended, reconnecting")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// consume opens one Consume stream for examples.crud.v1.ModelEvent and audits every event it
// receives until the stream ends (ctx cancelled, Catalyst not yet up, or Catalyst restarting) —
// returning that error to run's reconnect loop.
func consume(ctx context.Context, logger chassis.Logger, client acConnect.ConsumerClient) error {
	stream, err := client.Consume(ctx, connect.NewRequest(&acv1.ConsumeRequest{
		Message: &acv1.CloudEvent{Source: serviceName, Type: modelEventType},
	}))
	if err != nil {
		return err
	}
	logger.WithField("event_type", modelEventType).Info("crud-audit consume stream open")

	for stream.Receive() {
		event := stream.Msg().GetMessage()
		if event == nil || event.GetType() != modelEventType {
			continue
		}
		audit(ctx, logger, event)
	}
	return stream.Err()
}

// audit decodes one ModelEvent and records it as its own WideEvent — chassis.StartSpan is the
// same manual-span pattern services/examples/crud-event/service/rpc.go uses for its own request
// spans, just triggered by a consumed event instead of an inbound RPC. A malformed payload is
// logged and skipped, never fatal to the consume loop — one bad event must not take the auditor
// itself down.
func audit(ctx context.Context, logger chassis.Logger, event *acv1.CloudEvent) {
	var payload crudv1.ModelEvent
	if err := protojson.Unmarshal([]byte(event.GetTextData()), &payload); err != nil {
		logger.WithField("error", err.Error()).WithField("event_id", event.GetId()).
			Error("failed to unmarshal ModelEvent; skipping audit")
		return
	}

	op := auditSpanName(payload.GetOperation())
	ctx, span := chassis.StartSpan(ctx, "crud-audit."+op)
	span.SetBusinessAttribute("operation", payload.GetOperation().String())
	span.SetBusinessAttribute("model_id", payload.GetModel().GetId())
	span.SetBusinessAttribute("first_name", payload.GetModel().GetFirstName())
	span.SetBusinessAttribute("last_name", payload.GetModel().GetLastName())
	span.SetBusinessAttribute("source_event_id", event.GetId())
	span.SetBusinessAttribute("source", event.GetSource())
	if lag, ok := eventLag(event); ok {
		span.SetRuntimeAttribute("consume_lag_ms", strconv.FormatInt(lag.Milliseconds(), 10))
	}

	logger.WithContext(ctx).
		WithField("operation", payload.GetOperation().String()).
		WithField("model_id", payload.GetModel().GetId()).
		WithField("source", event.GetSource()).
		Info("audited model event")
	span.End(nil)
}

// auditSpanName maps an Operation to the lowercase word crud/crud-event's own span names already
// use (crud.create, crud-event.read, ...), so crud-audit.create/read/update/delete read
// consistently with them in Beacon.
func auditSpanName(op crudv1.Operation) string {
	switch op {
	case crudv1.Operation_OPERATION_CREATE:
		return "create"
	case crudv1.Operation_OPERATION_READ:
		return "read"
	case crudv1.Operation_OPERATION_UPDATE:
		return "update"
	case crudv1.Operation_OPERATION_DELETE:
		return "delete"
	default:
		return "unknown"
	}
}

// eventLag reads the "time" CloudEvent attribute crud-event's own events.go stamps on every
// ModelEvent and returns how long it took this consumer to receive it — a small but genuinely
// useful audit signal (a growing lag is a broker or consumer problem worth seeing before it
// becomes an outage), not available from the event's own payload.
func eventLag(event *acv1.CloudEvent) (time.Duration, bool) {
	ts := event.GetAttributes()["time"].GetCeTimestamp()
	if ts == nil {
		return 0, false
	}
	return time.Since(ts.AsTime()), true
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c), required for Connect
// streaming against Catalyst's plain-TCP server — same construction every other Catalyst client
// in this repo uses (services/examples/producer/main.go,
// services/examples/crud-event/service/events.go, ...).
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
