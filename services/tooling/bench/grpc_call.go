// This file implements bench://grpc-call@v1, the one built-in executor Phase 3
// ships: resolve `with.service`/`with.method` to a live address (discovery.go), POST
// a Connect-JSON request built from `with.request`, and evaluate `expect:` against
// the JSON response.
//
// Making the call itself needs nothing beyond plain net/http: a unary Connect RPC
// over JSON is just `POST http://<host:port>/<fully.qualified.Service>/<Method>`
// with `Content-Type: application/json` and a JSON body matching the request
// message's fields, returning a JSON body matching the response message on success.
// No generated Go client, gRPC reflection, or dynamic protobuf descriptors required
// — see the brief / docs/website/content/docs/architecture/bench-workflow-engine.md.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// grpcCallExecutor is the Executor behind bench://grpc-call@v1.
type grpcCallExecutor struct {
	resolver   ServiceResolver
	httpClient *http.Client
}

// NewGrpcCallExecutor builds the bench://grpc-call@v1 executor. The HTTP client
// uses the same h2c-over-plaintext transport (golang.org/x/net/http2, AllowHTTP)
// every other Draft-to-Draft client in this repo uses (pkg/chassis/builder.go,
// services/core/auth) — necessary because the target is an ordinary Draft/Connect
// service, which speaks HTTP/2 without TLS internally.
func NewGrpcCallExecutor(resolver ServiceResolver) Executor {
	return &grpcCallExecutor{
		resolver: resolver,
		httpClient: &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		},
	}
}

func (e *grpcCallExecutor) Execute(ctx context.Context, step *workflowv1.Step, with *structpb.Struct) (*ExecutionResult, error) {
	service := with.GetFields()["service"].GetStringValue()
	method := with.GetFields()["method"].GetStringValue()
	if service == "" || method == "" {
		return nil, fmt.Errorf("grpc-call: with.service and with.method are both required")
	}

	body, err := requestBody(with.GetFields()["request"])
	if err != nil {
		return nil, fmt.Errorf("grpc-call: with.request: %w", err)
	}

	addr, err := e.resolver.Resolve(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("grpc-call: %w", err)
	}
	url := fmt.Sprintf("http://%s/%s/%s", addr, service, method)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("grpc-call: building request to %s: %w", url, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("grpc-call: calling %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grpc-call: reading response from %s: %w", url, err)
	}

	respMap := map[string]interface{}{}
	if len(bytes.TrimSpace(respBody)) > 0 {
		if err := json.Unmarshal(respBody, &respMap); err != nil {
			return nil, fmt.Errorf("grpc-call: response from %s was not valid JSON: %w (body: %s)", url, err, truncate(respBody, 500))
		}
	}
	resultStruct, err := structpb.NewStruct(respMap)
	if err != nil {
		return nil, fmt.Errorf("grpc-call: response from %s could not be represented as a struct: %w", url, err)
	}

	statusOK := resp.StatusCode >= 200 && resp.StatusCode < 300

	var reasons []string
	if expect := step.GetExpect(); expect != nil {
		if statusField, ok := expect.GetFields()["status"]; ok {
			wantStatus := statusField.GetStringValue()
			switch wantStatus {
			case "", "OK":
				if !statusOK {
					reasons = append(reasons, fmt.Sprintf("expect.status: OK, got HTTP %d from %s (body: %s)", resp.StatusCode, url, truncate(respBody, 500)))
				}
			default:
				reasons = append(reasons, fmt.Sprintf("expect.status: %q is not a supported assertion (only \"OK\" is implemented)", wantStatus))
			}
		}
		if bodyField, ok := expect.GetFields()["body"]; ok {
			reasons = append(reasons, evaluateBodyExpect("body", bodyField.GetStructValue(), respMap)...)
		}
	}

	detail, detailErr := structpb.NewStruct(map[string]interface{}{
		"address":     addr,
		"url":         url,
		"http_status": resp.StatusCode,
	})
	if detailErr != nil {
		// Unreachable in practice (a string and an int always convert), but
		// don't let a detail-building failure mask the step's real outcome.
		detail = nil
	}

	return &ExecutionResult{
		Result:        resultStruct,
		Passed:        len(reasons) == 0,
		FailureReason: strings.Join(reasons, "; "),
		Detail:        detail,
	}, nil
}

// requestBody marshals with.request (a *structpb.Value wrapping a struct, or
// unset) into the JSON body to POST. An unset/null request sends "{}", matching a
// Connect unary call with an empty message.
func requestBody(request *structpb.Value) ([]byte, error) {
	if request == nil || request.GetStructValue() == nil {
		return []byte("{}"), nil
	}
	return protojson.Marshal(request.GetStructValue())
}

// evaluateBodyExpect recursively walks an `expect.body` struct against the parsed
// JSON response. A struct is treated as a leaf assertion (evaluated via
// evaluateLeaf) as soon as it has an "exists", "equals", or "matches" key;
// otherwise its keys are field names to recurse into, mirroring the response
// shape — e.g. `body: { name: { first_name: { equals: "Ada" } } }` asserts against
// response.name.first_name.
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

// valuesEqual compares a JSON-decoded actual value (string/float64/bool/nil/
// map[string]interface{}/[]interface{}, from encoding/json) against an expected
// value decoded the same way (structpb.Value.AsInterface() produces the identical
// shape), so a direct type-and-value comparison is safe.
func valuesEqual(actual, want interface{}) bool {
	actualJSON, err1 := json.Marshal(actual)
	wantJSON, err2 := json.Marshal(want)
	if err1 != nil || err2 != nil {
		return false
	}
	return bytes.Equal(actualJSON, wantJSON)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
