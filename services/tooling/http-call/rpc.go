// This file implements StepExecutor.Execute (see
// api/tooling/step_executor/v1/service.proto) — the fixed, minimal contract
// every Garage plugin implements, called directly by whatever resolved this
// plugin (Bench's garage:// executor, garage_plugin.go).
//
// What Execute does: makes one plain HTTP request from config
// (method/url/headers/body/timeout), returns status/headers/body as the
// step's result, and — if config.expect is set — evaluates status/body
// assertions against the response before reporting success.
//
// Deliberate design choice, not an oversight: assertions live under this
// plugin's own with.expect, not the workflow's top-level expect: keyword
// bench://grpc-call@v1 (services/tooling/bench/grpc_call.go) uses.
// StepRequest (the fixed contract every plugin implements) has no expect
// field — only config and context are passed to a plugin — and extending
// that shared contract for one plugin's benefit would break the doc's own
// framing of it as fixed and minimal
// (docs/website/content/docs/architecture/garage-plugin-repository.md,
// "What a plugin is"). Scoping assertions under this plugin's own with:
// block instead keeps that contract untouched and matches "everything else
// is up to the plugin" — the cost is that an http-call step's assertions
// use different YAML placement than a grpc-call step's, which is called
// out here and in the plugin's own config_schema description rather than
// left for a workflow author to discover by trial and error.
//
// The assertion grammar itself (exists/equals/matches, deep-equal via
// JSON-marshal comparison) is a deliberate duplicate of
// grpc_call.go's evaluateBodyExpect/evaluateLeaf/valuesEqual — this package
// can't import bench's (those are unexported, and even if they weren't,
// services/tooling/bench and services/tooling/http-call are separate Go
// modules with no shared dependency between them). See
// docs/website/content/docs/architecture/bench-workflow-engine.md's
// "Connecting a plugin registry" section for this repo's established
// precedent of duplicating small logic between tooling services rather
// than inventing a shared package neither module can import from the
// other.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	stepexecutorv1connect "github.com/steady-bytes/draft/api/tooling/step_executor/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// defaultTimeout is used when config.timeout is unset.
const defaultTimeout = 10 * time.Second

type (
	Handler interface {
		chassis.RPCRegistrar
		stepexecutorv1connect.StepExecutorHandler
	}
	handler struct {
		logger     chassis.Logger
		httpClient *http.Client
	}
)

func NewHandler(logger chassis.Logger, httpClient *http.Client) Handler {
	return &handler{logger: logger, httpClient: httpClient}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := stepexecutorv1connect.NewStepExecutorHandler(h)
	server.AddHandler(pattern, handler, true)
}

// Execute makes one HTTP request from config and evaluates config.expect
// (if set) against the response. A returned Go error is reserved for the
// RPC call itself being malformed (a nil Config); every other failure (a
// missing method/url, a transport error, a failed assertion) is reported
// through StepResponse.Success/Error, per the design doc's contract.
func (h *handler) Execute(ctx context.Context, req *connect.Request[stepexecutorv1.StepRequest]) (*connect.Response[stepexecutorv1.StepResponse], error) {
	msg := req.Msg
	if msg.GetConfig() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config is required"))
	}

	call, err := parseConfig(msg.GetConfig())
	if err != nil {
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, call.timeout)
	defer cancel()

	status, headers, bodyBytes, err := doRequest(reqCtx, h.httpClient, call)
	if err != nil {
		h.logger.WithError(err).WithField("url", call.url).Warn("http-call step failed to complete the request")
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	parsedBody, bodyIsJSON := decodeBody(bodyBytes)

	var reasons []string
	if call.expect != nil {
		reasons = evaluateExpect(call.expect, status, parsedBody, bodyIsJSON)
	} else if status < 200 || status >= 300 {
		reasons = append(reasons, fmt.Sprintf("HTTP %d (no expect: configured, default success is any 2xx)", status))
	}

	result, err := buildResult(status, headers, parsedBody, bodyIsJSON, bodyBytes)
	if err != nil {
		h.logger.WithError(err).Warn("failed to build result struct; returning a minimal result")
		result, _ = structpb.NewStruct(map[string]interface{}{"status": status})
	}

	return connect.NewResponse(&stepexecutorv1.StepResponse{
		Success: len(reasons) == 0,
		Result:  result,
		Error:   strings.Join(reasons, "; "),
	}), nil
}

// httpCall is the parsed, validated form of config.
type httpCall struct {
	method  string
	url     string
	headers map[string]string
	body    []byte
	timeout time.Duration
	expect  *structpb.Struct
}

func parseConfig(config *structpb.Struct) (*httpCall, error) {
	fields := config.GetFields()

	method := strings.ToUpper(fields["method"].GetStringValue())
	if method == "" {
		return nil, errors.New("config.method is required")
	}
	url := fields["url"].GetStringValue()
	if url == "" {
		return nil, errors.New("config.url is required")
	}

	headers := map[string]string{}
	for k, v := range fields["headers"].GetStructValue().GetFields() {
		headers[k] = v.GetStringValue()
	}

	body, err := requestBody(fields["body"])
	if err != nil {
		return nil, fmt.Errorf("config.body: %w", err)
	}

	timeout := defaultTimeout
	if timeoutVal := fields["timeout"].GetStringValue(); timeoutVal != "" {
		d, err := time.ParseDuration(timeoutVal)
		if err != nil {
			return nil, fmt.Errorf("config.timeout: invalid duration %q: %w", timeoutVal, err)
		}
		timeout = d
	}

	var expect *structpb.Struct
	if expectVal, ok := fields["expect"]; ok {
		expect = expectVal.GetStructValue()
	}

	return &httpCall{
		method:  method,
		url:     url,
		headers: headers,
		body:    body,
		timeout: timeout,
		expect:  expect,
	}, nil
}

// requestBody marshals config.body into request bytes: unset -> nil (no
// body); a string -> sent verbatim; anything else (object/array/number/
// bool) -> JSON-encoded. Mirrors
// services/tooling/bench/grpc_call.go's requestBody in spirit (unset means
// no/empty body), generalized beyond "always JSON" since a generic HTTP
// call plugin has to support non-JSON payloads too.
func requestBody(v *structpb.Value) ([]byte, error) {
	if v == nil || v.GetKind() == nil {
		return nil, nil
	}
	if s, ok := v.GetKind().(*structpb.Value_StringValue); ok {
		return []byte(s.StringValue), nil
	}
	return json.Marshal(v.AsInterface())
}

func doRequest(ctx context.Context, client *http.Client, call *httpCall) (status int, headers http.Header, body []byte, err error) {
	var bodyReader io.Reader
	if call.body != nil {
		bodyReader = bytes.NewReader(call.body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, call.method, call.url, bodyReader)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("building request to %s: %w", call.url, err)
	}
	for k, v := range call.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("calling %s %s: %w", call.method, call.url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("reading response from %s: %w", call.url, err)
	}

	return resp.StatusCode, resp.Header, respBody, nil
}

// decodeBody tries to parse body as JSON (object, array, or scalar all
// count); falls back to the raw string when it isn't. The bool return says
// which happened, since a JSON scalar (e.g. the literal 4-byte body
// "true") and a non-JSON string need to be told apart for result-building
// and body assertions.
func decodeBody(body []byte) (parsed interface{}, isJSON bool) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, false
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return string(body), false
	}
	return v, true
}

func buildResult(status int, headers http.Header, parsedBody interface{}, bodyIsJSON bool, rawBody []byte) (*structpb.Struct, error) {
	headerMap := map[string]interface{}{}
	for k := range headers {
		headerMap[k] = headers.Get(k)
	}

	var bodyOut interface{}
	if bodyIsJSON {
		bodyOut = parsedBody
	} else if len(rawBody) > 0 {
		bodyOut = string(rawBody)
	}

	return structpb.NewStruct(map[string]interface{}{
		"status":  status,
		"headers": headerMap,
		"body":    bodyOut,
	})
}

// evaluateExpect checks expect.status and expect.body against the response,
// mirroring services/tooling/bench/grpc_call.go's own expect evaluation —
// see this file's header comment for why it's duplicated rather than
// shared, and why it lives under with.expect instead of a top-level
// expect: key.
func evaluateExpect(expect *structpb.Struct, status int, parsedBody interface{}, bodyIsJSON bool) []string {
	var reasons []string

	if statusField, ok := expect.GetFields()["status"]; ok {
		reasons = append(reasons, evaluateStatus(statusField, status)...)
	}

	if bodyField, ok := expect.GetFields()["body"]; ok {
		bodyMap, _ := parsedBody.(map[string]interface{})
		if !bodyIsJSON {
			reasons = append(reasons, "expect.body: response body was not valid JSON")
		} else {
			reasons = append(reasons, evaluateBodyExpect("body", bodyField.GetStructValue(), bodyMap)...)
		}
	}

	return reasons
}

// evaluateStatus accepts either the string "OK" (any 2xx, matching
// grpc_call.go's own convention for consistency across executors) or an
// exact numeric status code — the latter is the generalization a generic
// HTTP-call plugin needs beyond grpc_call.go's binary OK/not-OK, since
// asserting a specific error status (e.g. 404) is a normal thing to want
// to test.
func evaluateStatus(statusField *structpb.Value, actual int) []string {
	switch v := statusField.AsInterface().(type) {
	case string:
		if v == "" || strings.EqualFold(v, "OK") {
			if actual < 200 || actual >= 300 {
				return []string{fmt.Sprintf("expect.status: OK, got HTTP %d", actual)}
			}
			return nil
		}
		return []string{fmt.Sprintf("expect.status: %q is not a supported assertion (use \"OK\" or an exact status code)", v)}
	case float64:
		want := int(v)
		if actual != want {
			return []string{fmt.Sprintf("expect.status: %d, got HTTP %d", want, actual)}
		}
		return nil
	default:
		return []string{fmt.Sprintf("expect.status: unsupported value %v (use \"OK\" or an exact status code)", v)}
	}
}

// evaluateBodyExpect recursively walks an expect.body struct against the
// parsed JSON response body — verbatim logic from
// services/tooling/bench/grpc_call.go's function of the same name (see
// that file for the fuller doc comment on the recursion/leaf-detection
// shape); duplicated here per this file's header comment.
func evaluateBodyExpect(path string, assertion *structpb.Struct, actual map[string]interface{}) []string {
	var problems []string
	for field, av := range assertion.GetFields() {
		fieldPath := path + "." + field
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
