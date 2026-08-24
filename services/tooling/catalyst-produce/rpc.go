// This file implements StepExecutor.Execute (see
// api/tooling/step_executor/v1/service.proto) — the fixed, minimal contract
// every Garage plugin implements, called directly by whatever resolved this
// plugin (Bench's garage:// executor, garage_plugin.go).
//
// What Execute does: builds a CloudEvent from config.event_type/source/
// subject/data and sends it on a single long-lived Produce stream to
// Catalyst, opened once at construction (NewHandler) and reused for the
// life of the process — the same stream-reuse shape
// services/tooling/bench/catalyst.go's catalystPublisher and
// services/examples/producer/main.go both already use, and for the same
// reason (a Connect bidi stream doesn't dial until the first Send, so
// opening it early is cheap even if Catalyst isn't up yet).
//
// Unlike Bench's own catalystPublisher, which treats a publish failure as
// best-effort/non-fatal (those events are a secondary observability
// channel — GetRun remains the source of truth regardless), a Send failure
// here fails the step: publishing the configured event *is* this plugin's
// entire deliverable, not a side effect of something else succeeding.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	stepexecutorv1connect "github.com/steady-bytes/draft/api/tooling/step_executor/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultCatalystAddress matches services/tooling/bench/catalyst.go's own
// fallback for the same config key.
const defaultCatalystAddress = "http://localhost:2220"

// defaultSource is used when config.source is unset.
const defaultSource = "/plugins/catalyst-produce"

type (
	Handler interface {
		chassis.RPCRegistrar
		stepexecutorv1connect.StepExecutorHandler
	}
	handler struct {
		logger chassis.Logger
		mu     sync.Mutex
		stream *connect.BidiStreamForClient[acv1.ProduceRequest, acv1.ProduceResponse]
	}
)

// NewHandler opens the Produce stream against catalystAddr immediately —
// ctx should outlive every Execute call this handler will ever serve;
// main.go passes one tied to chassis.Closer(), so the stream is torn down
// on shutdown, not per-call, mirroring catalystPublisher's own
// construction in services/tooling/bench/catalyst.go.
func NewHandler(ctx context.Context, logger chassis.Logger, catalystAddr string) Handler {
	client := acConnect.NewProducerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	return &handler{logger: logger, stream: client.Produce(ctx)}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := stepexecutorv1connect.NewStepExecutorHandler(h)
	server.AddHandler(pattern, handler, true)
}

// Execute builds and publishes one CloudEvent from config. A returned Go
// error is reserved for the RPC call itself being malformed (a nil
// Config); every other failure (a missing event_type, a Send error) is
// reported through StepResponse.Success/Error, per the design doc's
// contract.
func (h *handler) Execute(ctx context.Context, req *connect.Request[stepexecutorv1.StepRequest]) (*connect.Response[stepexecutorv1.StepResponse], error) {
	msg := req.Msg
	if msg.GetConfig() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config is required"))
	}

	eventType, source, subject, data, delay, err := parseConfig(msg.GetConfig())
	if err != nil {
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	// config.delay exists for exactly one reason: Catalyst's Consume has no
	// event replay (see docs/website/content/docs/architecture/
	// garage-plugin-repository.md's note on this same plugin's design) — a
	// subscriber that opens its stream even a moment after this event was
	// sent has permanently missed it, no buffering or re-delivery. A
	// workflow that produces and consumes the same event in one run can
	// only ever work if the two steps run as concurrent siblings (Bench's
	// "empty depends_on defaults to the previous step" rule means true
	// independence needs a shared parent step — see
	// services/tooling/bench/workflows/catalyst-produce-consume-e2e.yaml),
	// and even then the consumer's stream-open round trip is real added
	// latency the producer's already-open stream doesn't pay. delay gives
	// the consumer a deterministic head start instead of hoping a bare race
	// resolves the right way.
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return connect.NewResponse(&stepexecutorv1.StepResponse{
				Success: false,
				Error:   fmt.Sprintf("context canceled while waiting out config.delay: %s", ctx.Err()),
			}), nil
		}
	}

	event, err := buildEvent(eventType, source, subject, data)
	if err != nil {
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	if err := h.send(event); err != nil {
		h.logger.WithError(err).WithField("event_type", eventType).Warn("catalyst-produce step failed to publish")
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to publish event: %s", err),
		}), nil
	}

	result, err := structpb.NewStruct(map[string]interface{}{
		"event_id":     event.GetId(),
		"type":         event.GetType(),
		"source":       event.GetSource(),
		"published_at": timestamppb.Now().AsTime().Format(timeFormat),
	})
	if err != nil {
		h.logger.WithError(err).Warn("failed to build result struct; returning empty result")
		result = &structpb.Struct{}
	}

	return connect.NewResponse(&stepexecutorv1.StepResponse{
		Success: true,
		Result:  result,
	}), nil
}

// timeFormat matches time.RFC3339Nano without importing "time" solely for
// the constant.
const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

// parseConfig extracts event_type (required), source (optional, defaults
// to defaultSource), subject (optional), data (optional, defaults to an
// empty object), and delay (optional, defaults to 0 — no wait) — see
// manifest.go's config_schema for the shape each matches.
func parseConfig(config *structpb.Struct) (eventType, source, subject string, data map[string]interface{}, delay time.Duration, err error) {
	fields := config.GetFields()

	eventTypeVal, ok := fields["event_type"]
	if !ok || eventTypeVal.GetStringValue() == "" {
		return "", "", "", nil, 0, errors.New("config.event_type is required")
	}
	eventType = eventTypeVal.GetStringValue()

	source = defaultSource
	if sourceVal, ok := fields["source"]; ok && sourceVal.GetStringValue() != "" {
		source = sourceVal.GetStringValue()
	}

	if subjectVal, ok := fields["subject"]; ok {
		subject = subjectVal.GetStringValue()
	}

	data = map[string]interface{}{}
	if dataVal, ok := fields["data"]; ok && dataVal.GetStructValue() != nil {
		data = dataVal.GetStructValue().AsMap()
	}

	if delayVal := fields["delay"].GetStringValue(); delayVal != "" {
		delay, err = time.ParseDuration(delayVal)
		if err != nil {
			return "", "", "", nil, 0, fmt.Errorf("config.delay: invalid duration %q: %w", delayVal, err)
		}
	}

	return eventType, source, subject, data, delay, nil
}

// buildEvent constructs the CloudEvent to send, matching the shape
// services/tooling/bench/catalyst.go's runEvent/stepEvent build (a JSON
// text-data body, a "time" attribute, and — when set — a "subject"
// attribute).
func buildEvent(eventType, source, subject string, data map[string]interface{}) (*acv1.CloudEvent, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("config.data could not be marshaled to JSON: %w", err)
	}

	attrs := map[string]*acv1.CloudEvent_CloudEventAttributeValue{
		"time": {Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{CeTimestamp: timestamppb.Now()}},
	}
	if subject != "" {
		attrs["subject"] = &acv1.CloudEvent_CloudEventAttributeValue{Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{CeString: subject}}
	}

	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      source,
		SpecVersion: "1.0",
		Type:        eventType,
		Data:        &acv1.CloudEvent_TextData{TextData: string(body)},
		Attributes:  attrs,
	}, nil
}

// send publishes event on the shared stream. A Connect bidi stream's Send
// is not safe for concurrent use, and concurrent workflow runs can call
// Execute at the same time — the same reason
// services/tooling/bench/catalyst.go's catalystPublisher.publish takes a
// mutex around its own Send call.
func (h *handler) send(event *acv1.CloudEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stream.Send(&acv1.ProduceRequest{Message: event})
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c),
// required for Connect streaming against Catalyst's plain-TCP server — same
// construction services/tooling/catalyst-consume/rpc.go and
// services/tooling/bench/catalyst.go both use.
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
