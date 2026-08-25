// This file implements StepExecutor.Execute (see
// api/tooling/step_executor/v1/service.proto) — the fixed, minimal contract
// every Garage plugin implements, called directly by whatever resolved this
// plugin (Bench's garage:// executor, garage_plugin.go).
//
// What Execute does: opens a Consume stream against Catalyst (the same raw
// Connect client services/examples/consumer/main.go demonstrates — see
// services/tooling/bench/catalyst.go's own file comment for why Catalyst's
// chassis.Broker interface isn't the right fit here either), waits for the
// next CloudEvent matching config.event_type, decodes its JSON payload, and
// copies the dot-paths named in config.fields into the step's result.
//
// Known Catalyst limitation this plugin inherits (see "Known issues" under
// Catalyst in docs/website/content/docs/architecture/core-services.md):
// Consume delivers each event to exactly one of the currently-open Consume
// streams, not to every one of them. If more than one consumer (another
// catalyst-consume step running concurrently, Bench's own future live-UI
// consumer, ...) is subscribed at the same moment, this step's wait can miss
// an event that went to a different subscriber instead — not something this
// plugin can work around client-side.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	stepexecutorv1connect "github.com/steady-bytes/draft/api/tooling/step_executor/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/types/known/structpb"
)

// defaultCatalystAddress matches services/tooling/bench/catalyst.go's own
// fallback for the same config key.
const defaultCatalystAddress = "http://localhost:2220"

// defaultWaitTimeout is used when config.timeout is unset — long enough for
// a normal test workflow's next step to actually happen, short enough that
// a misconfigured event_type fails a run in a reasonable time rather than
// hanging it.
const defaultWaitTimeout = 10 * time.Second

type (
	Handler interface {
		chassis.RPCRegistrar
		stepexecutorv1connect.StepExecutorHandler
	}
	handler struct {
		logger chassis.Logger
		// catalystAddr is read lazily (rather than captured once at
		// construction), same reasoning as slack-notify's webhookURL field:
		// a config reload or test double can vary it per call.
		catalystAddr func() string
	}
)

func NewHandler(logger chassis.Logger, catalystAddr func() string) Handler {
	return &handler{logger: logger, catalystAddr: catalystAddr}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := stepexecutorv1connect.NewStepExecutorHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

// Execute waits for the next Catalyst event matching config.event_type and
// extracts config.fields from its payload. A returned Go error is reserved
// for the RPC call itself being malformed (a nil Config); every other
// failure (a missing event_type, a stream error, a timeout) is reported
// through StepResponse.Success/Error, per the design doc's contract.
func (h *handler) Execute(ctx context.Context, req *connect.Request[stepexecutorv1.StepRequest]) (*connect.Response[stepexecutorv1.StepResponse], error) {
	msg := req.Msg
	if msg.GetConfig() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config is required"))
	}

	eventType, timeout, fields, err := parseConfig(msg.GetConfig())
	if err != nil {
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	event, err := waitForEvent(waitCtx, h.catalystAddr(), eventType)
	if err != nil {
		if waitCtx.Err() != nil && ctx.Err() == nil {
			err = fmt.Errorf("timed out after %s waiting for event type %q", timeout, eventType)
		}
		h.logger.WithError(err).WithField("event_type", eventType).Warn("catalyst-consume step did not complete")
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	result, err := buildResult(event, fields)
	if err != nil {
		h.logger.WithError(err).Warn("failed to build result struct from matched event; returning empty result")
		result = &structpb.Struct{}
	}

	return connect.NewResponse(&stepexecutorv1.StepResponse{
		Success: true,
		Result:  result,
	}), nil
}

// parseConfig extracts event_type (required), timeout (optional, defaults
// to defaultWaitTimeout), and fields (optional, defaults to empty — see
// manifest.go's config_schema for the shape each matches).
func parseConfig(config *structpb.Struct) (eventType string, timeout time.Duration, fields map[string]string, err error) {
	fieldsIn := config.GetFields()

	eventTypeVal, ok := fieldsIn["event_type"]
	if !ok || eventTypeVal.GetStringValue() == "" {
		return "", 0, nil, errors.New("config.event_type is required")
	}
	eventType = eventTypeVal.GetStringValue()

	timeout = defaultWaitTimeout
	if timeoutVal, ok := fieldsIn["timeout"]; ok && timeoutVal.GetStringValue() != "" {
		d, err := time.ParseDuration(timeoutVal.GetStringValue())
		if err != nil {
			return "", 0, nil, fmt.Errorf("config.timeout: invalid duration %q: %w", timeoutVal.GetStringValue(), err)
		}
		timeout = d
	}

	fields = map[string]string{}
	if fieldsVal, ok := fieldsIn["fields"]; ok {
		for outKey, pathVal := range fieldsVal.GetStructValue().GetFields() {
			if s := pathVal.GetStringValue(); s != "" {
				fields[outKey] = s
			}
		}
	}

	return eventType, timeout, fields, nil
}

// waitForEvent opens a Consume stream against catalystAddr and returns the
// first event whose Type matches eventType, or an error once ctx is done
// (timeout or cancellation) or the stream itself fails.
func waitForEvent(ctx context.Context, catalystAddr, eventType string) (*acv1.CloudEvent, error) {
	client := acConnect.NewConsumerClient(h2cClient(), catalystAddr, connect.WithGRPC())
	stream, err := client.Consume(ctx, connect.NewRequest(&acv1.ConsumeRequest{
		Message: &acv1.CloudEvent{
			Source: pluginName,
			Type:   eventType,
		},
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to open catalyst consume stream: %w", err)
	}

	for stream.Receive() {
		event := stream.Msg().GetMessage()
		if event == nil || event.GetType() != eventType {
			continue
		}
		return event, nil
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("catalyst consume stream ended: %w", err)
	}
	return nil, fmt.Errorf("catalyst consume stream closed before a matching event arrived")
}

// buildResult decodes event's JSON payload and copies each configured
// output_key -> dot-path into the step's result, alongside _event metadata
// and the full decoded payload (so a step that forgot to declare a field it
// needs isn't blocked from the raw data — the same "return everything, let
// templating pick" shape bench://grpc-call@v1's own Result already uses).
func buildResult(event *acv1.CloudEvent, fields map[string]string) (*structpb.Struct, error) {
	payload := map[string]interface{}{}
	if body := event.GetTextData(); body != "" {
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			return nil, fmt.Errorf("event payload was not valid JSON: %w", err)
		}
	}

	out := map[string]interface{}{
		"_event": map[string]interface{}{
			"type":    event.GetType(),
			"source":  event.GetSource(),
			"id":      event.GetId(),
			"subject": attrString(event, "subject"),
		},
		"payload": payload,
	}
	for outKey, path := range fields {
		if v, ok := extractPath(payload, path); ok {
			out[outKey] = v
		}
	}

	return structpb.NewStruct(out)
}

// extractPath walks a dot-separated path (e.g. "name.firstName") through a
// decoded JSON object, matching the shape any protojson-marshaled Draft
// event payload actually has (camelCase field names, nested objects) —
// confirmed live against Bench's own RunEvent/StepEvent payloads
// (services/tooling/bench/catalyst.go) during this plugin's development.
func extractPath(payload map[string]interface{}, path string) (interface{}, bool) {
	var cur interface{} = payload
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		v, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// attrString reads a CloudEvent's string-typed attribute (e.g. "subject"),
// matching the shape services/tooling/bench/catalyst.go's ceStringAttr
// writes. Returns "" if unset or a different attribute kind.
func attrString(event *acv1.CloudEvent, key string) string {
	return event.GetAttributes()[key].GetCeString()
}

// h2cClient returns an HTTP client that speaks HTTP/2 cleartext (h2c),
// required for Connect streaming against Catalyst's plain-TCP server — same
// construction services/examples/consumer/main.go and
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
