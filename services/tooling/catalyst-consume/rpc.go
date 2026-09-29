// This file implements StepExecutor.Execute (see
// api/tooling/step_executor/v1/service.proto) — the fixed, minimal contract
// every Foundry plugin implements, called directly by whatever resolved this
// plugin (Bench's foundry:// executor, foundry_plugin.go).
//
// What Execute does: opens a Consume stream against Catalyst (the same raw
// Connect client services/examples/consumer/main.go demonstrates — see
// services/tooling/bench/catalyst.go's own file comment for why Catalyst's
// chassis.Broker interface isn't the right fit here either), waits for the
// next CloudEvent matching config.event_type, decodes its JSON payload,
// copies the dot-paths named in config.fields into the step's result, and —
// if config.expect is set — evaluates assertions against that decoded
// payload before reporting success, the same with.expect convention
// services/tooling/http-call/rpc.go documents and implements (assertions
// live under this plugin's own config rather than the workflow's top-level
// expect: keyword bench://grpc-call@v1 uses, for the same reason given
// there: StepRequest's contract is fixed and minimal, so a plugin that
// wants assertions scopes them under its own with: block instead). The
// evaluateExpect/evaluateBodyExpect/evaluateLeaf/valuesEqual functions below
// are a deliberate duplicate of http-call's and grpc_call.go's — see
// http-call/rpc.go's header comment for why duplicating this small amount
// of logic is this repo's established precedent over a shared package
// neither module could import from the others anyway (three separate Go
// modules).
//
// Consume used to deliver each event to exactly one of the currently-open
// Consume streams, not to every one of them (see "Known issues" under
// Catalyst in docs/website/content/docs/architecture/core-services.md,
// fixed 2026-09-27) — if more than one consumer (another catalyst-consume
// step running concurrently, Bench's own future live-UI consumer, ...) was
// subscribed at the same moment, this step's wait could miss an event that
// went to a different subscriber instead. Every open Consume subscriber now
// gets its own delivery, so that's no longer a caveat.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
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

	eventType, timeout, fields, expect, err := parseConfig(msg.GetConfig())
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

	payload, err := decodePayload(event)
	if err != nil {
		h.logger.WithError(err).WithField("event_type", eventType).Warn("catalyst-consume step matched an event with an undecodable payload")
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	var reasons []string
	if expect != nil {
		reasons = evaluateBodyExpect("", expect, payload)
	}

	result, err := buildResult(event, payload, fields)
	if err != nil {
		h.logger.WithError(err).Warn("failed to build result struct from matched event; returning empty result")
		result = &structpb.Struct{}
	}

	return connect.NewResponse(&stepexecutorv1.StepResponse{
		Success: len(reasons) == 0,
		Result:  result,
		Error:   strings.Join(reasons, "; "),
	}), nil
}

// parseConfig extracts event_type (required), timeout (optional, defaults
// to defaultWaitTimeout), fields (optional, defaults to empty), and expect
// (optional, nil if unset) — see manifest.go's config_schema for the shape
// each matches.
func parseConfig(config *structpb.Struct) (eventType string, timeout time.Duration, fields map[string]string, expect *structpb.Struct, err error) {
	fieldsIn := config.GetFields()

	eventTypeVal, ok := fieldsIn["event_type"]
	if !ok || eventTypeVal.GetStringValue() == "" {
		return "", 0, nil, nil, errors.New("config.event_type is required")
	}
	eventType = eventTypeVal.GetStringValue()

	timeout = defaultWaitTimeout
	if timeoutVal, ok := fieldsIn["timeout"]; ok && timeoutVal.GetStringValue() != "" {
		d, err := time.ParseDuration(timeoutVal.GetStringValue())
		if err != nil {
			return "", 0, nil, nil, fmt.Errorf("config.timeout: invalid duration %q: %w", timeoutVal.GetStringValue(), err)
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

	if expectVal, ok := fieldsIn["expect"]; ok {
		expect = expectVal.GetStructValue()
	}

	return eventType, timeout, fields, expect, nil
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

// decodePayload decodes event's JSON text payload once, shared by buildResult
// (below) and Execute's config.expect evaluation, which both need the same
// decoded map rather than re-parsing it twice.
func decodePayload(event *acv1.CloudEvent) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if body := event.GetTextData(); body != "" {
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			return nil, fmt.Errorf("event payload was not valid JSON: %w", err)
		}
	}
	return payload, nil
}

// buildResult copies each configured output_key -> dot-path into the step's
// result, alongside _event metadata and the full decoded payload (so a step
// that forgot to declare a field it needs isn't blocked from the raw data —
// the same "return everything, let templating pick" shape
// bench://grpc-call@v1's own Result already uses).
func buildResult(event *acv1.CloudEvent, payload map[string]interface{}, fields map[string]string) (*structpb.Struct, error) {
	out := map[string]interface{}{
		"_event": map[string]interface{}{
			"type":    event.GetType(),
			"source":  event.GetSource(),
			"id":      event.GetId(),
			"subject": attrString(event, "subject"),
			// Surfaced for correlation, not auto-applied to this step's own
			// span: catalyst-consume's Execute is already correctly parented
			// to whatever called it (bench, via foundry_plugin.go's
			// NewTraceClientInterceptor) — that's a real, useful relationship
			// in its own right ("which workflow step triggered this"),
			// distinct from "who originally produced the event," which this
			// is. Forcing one parent_span_id to represent both would destroy
			// the first to gain the second; real tracing systems solve this
			// with span links (a secondary reference), which WideEvent's
			// schema doesn't have yet. A workflow author who wants to jump to
			// the producing trace can read this directly.
			"traceparent": attrString(event, chassis.CloudEventTraceParentAttributeKey),
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

// evaluateBodyExpect recursively walks a config.expect struct against the
// matched event's decoded JSON payload — logic duplicated from
// services/tooling/http-call/rpc.go's function of the same name (itself
// duplicated from services/tooling/bench/grpc_call.go); see this file's
// header comment for why it's duplicated here too rather than shared. One
// deliberate difference from http-call's version: http-call's assertions
// live under with.expect.body (an extra level, since a response has both a
// status and a body to assert on), so it calls this with path="body" and
// every message below ends up "expect.body.<field>: ...". This plugin's
// assertions are directly under with.expect (there's no second thing to
// assert alongside the payload), so Execute calls this with path="" — the
// path!="" guard below is what keeps that from rendering "expect..<field>"
// (or, worse, passing path="expect" to sidestep that would have rendered
// "expect.expect.<field>": the "expect." each message below is hardcoded,
// not derived from path, exactly as in http-call's copy).
func evaluateBodyExpect(path string, assertion *structpb.Struct, actual map[string]interface{}) []string {
	var problems []string
	for field, av := range assertion.GetFields() {
		fieldPath := field
		if path != "" {
			fieldPath = path + "." + field
		}
		sub := av.GetStructValue()
		if sub == nil {
			problems = append(problems, fmt.Sprintf("expect.%s: assertion must be an object", fieldPath))
			continue
		}

		actualVal, exists := actual[field]

		if isLeafAssertion(sub) {
			problems = append(problems, evaluateLeaf(fieldPath, sub, actualVal, exists)...)
			continue
		}

		nested, ok := actualVal.(map[string]interface{})
		if !exists || !ok {
			nested = nil
		}
		problems = append(problems, evaluateBodyExpect(fieldPath, sub, nested)...)
	}
	return problems
}

func isLeafAssertion(s *structpb.Struct) bool {
	fields := s.GetFields()
	_, hasExists := fields["exists"]
	_, hasEquals := fields["equals"]
	_, hasMatches := fields["matches"]
	return hasExists || hasEquals || hasMatches
}

func evaluateLeaf(path string, assertion *structpb.Struct, actual interface{}, exists bool) []string {
	var problems []string

	if existsField, ok := assertion.GetFields()["exists"]; ok {
		want := existsField.GetBoolValue()
		if exists != want {
			problems = append(problems, fmt.Sprintf("expect.%s: exists = %v, want %v", path, exists, want))
		}
	}

	if equalsField, ok := assertion.GetFields()["equals"]; ok {
		want := equalsField.AsInterface()
		if !exists {
			problems = append(problems, fmt.Sprintf("expect.%s: field does not exist, want equals %v", path, want))
		} else if !valuesEqual(actual, want) {
			problems = append(problems, fmt.Sprintf("expect.%s: got %v, want %v", path, actual, want))
		}
	}

	if matchesField, ok := assertion.GetFields()["matches"]; ok {
		pattern := matchesField.GetStringValue()
		str, isStr := actual.(string)
		switch {
		case !exists:
			problems = append(problems, fmt.Sprintf("expect.%s: field does not exist, want matches %q", path, pattern))
		case !isStr:
			problems = append(problems, fmt.Sprintf("expect.%s: matches requires a string field, got %T", path, actual))
		default:
			re, err := regexp.Compile(pattern)
			if err != nil {
				problems = append(problems, fmt.Sprintf("expect.%s: invalid matches pattern %q: %v", path, pattern, err))
			} else if !re.MatchString(str) {
				problems = append(problems, fmt.Sprintf("expect.%s: %q does not match pattern %q", path, str, pattern))
			}
		}
	}

	return problems
}

func valuesEqual(actual, want interface{}) bool {
	actualJSON, err1 := json.Marshal(actual)
	wantJSON, err2 := json.Marshal(want)
	if err1 != nil || err2 != nil {
		return false
	}
	return bytes.Equal(actualJSON, wantJSON)
}
