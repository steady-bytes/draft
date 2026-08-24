// This file implements StepExecutor.Execute (see
// api/tooling/step_executor/v1/service.proto) — the fixed, minimal contract
// every Garage plugin implements, called directly by whatever resolved this
// plugin (Bench's garage:// executor, garage_plugin.go).
//
// What Execute does, and why this plugin exists at all alongside Bench's
// own built-in bench://grpc-call@v1 (services/tooling/bench/grpc_call.go):
// the built-in executor only calls Draft/Connect services, over plain
// HTTP+JSON, resolved by name through Blueprint — it has no generated
// stubs or reflection because every Draft service already speaks that
// exact JSON-over-HTTP/2 shape. This plugin covers the gap: an arbitrary
// external gRPC server (address + fully-qualified service/method), calling
// it with real gRPC framing over a connection dialed at request time,
// resolving the request/response message shape at runtime via the
// server's own reflection service (google.golang.org/grpc/reflection) —
// no generated Go client for the target service is available or assumed.
// This needs a dynamic-protobuf client, which is why this plugin (uniquely
// among this repo's Garage plugins) depends on
// github.com/jhump/protoreflect: hand-rolling FileDescriptor dependency
// resolution over the raw reflection RPC correctly (transitive imports,
// well-known types, diamond deps) is exactly what that library (also what
// grpcurl itself is built on) already does robustly — reimplementing it
// here would be real, easy-to-get-subtly-wrong engineering for a solved
// problem.
//
// expect: placement follows the same reasoning as
// services/tooling/http-call/rpc.go's file comment: config.expect, not the
// workflow's top-level expect: keyword, since StepRequest (the fixed
// contract every plugin implements) carries no expect field. The
// assertion grammar (exists/equals/matches) is the same duplicated logic
// as http-call's, adapted for a grpc status code + a decoded response
// message instead of an HTTP status + JSON body.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	stepexecutorv1connect "github.com/steady-bytes/draft/api/tooling/step_executor/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
		logger chassis.Logger
	}
)

func NewHandler(logger chassis.Logger) Handler {
	return &handler{logger: logger}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := stepexecutorv1connect.NewStepExecutorHandler(h)
	server.AddHandler(pattern, handler, true)
}

// Execute dials config.address, resolves config.service/method via server
// reflection, invokes it with config.request, and evaluates config.expect
// (if set) against the response. A returned Go error is reserved for the
// RPC call itself being malformed (a nil Config); every other failure (a
// missing field, a dial/reflection/call error, a failed assertion) is
// reported through StepResponse.Success/Error, per the design doc's
// contract.
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

	callCtx, cancel := context.WithTimeout(ctx, call.timeout)
	defer cancel()

	respMap, grpcErr := invoke(callCtx, call)

	code := status.Code(grpcErr)
	var reasons []string
	if call.expect != nil {
		reasons = evaluateExpect(call.expect, code, respMap)
	} else if grpcErr != nil {
		reasons = append(reasons, fmt.Sprintf("grpc call failed: %s", grpcErr))
	}

	result, resultErr := structpb.NewStruct(map[string]interface{}{"response": respMap})
	if resultErr != nil {
		h.logger.WithError(resultErr).Warn("failed to build result struct; returning an empty result")
		result = &structpb.Struct{}
	}

	return connect.NewResponse(&stepexecutorv1.StepResponse{
		Success: len(reasons) == 0,
		Result:  result,
		Error:   strings.Join(reasons, "; "),
	}), nil
}

// grpcCall is the parsed, validated form of config.
type grpcCall struct {
	address string
	service string
	method  string
	request map[string]interface{}
	useTLS  bool
	md      metadata.MD
	timeout time.Duration
	expect  *structpb.Struct
}

func parseConfig(config *structpb.Struct) (*grpcCall, error) {
	fields := config.GetFields()

	address := fields["address"].GetStringValue()
	if address == "" {
		return nil, errors.New("config.address is required")
	}
	service := fields["service"].GetStringValue()
	if service == "" {
		return nil, errors.New("config.service is required")
	}
	method := fields["method"].GetStringValue()
	if method == "" {
		return nil, errors.New("config.method is required")
	}

	request := map[string]interface{}{}
	if reqVal, ok := fields["request"]; ok && reqVal.GetStructValue() != nil {
		request = reqVal.GetStructValue().AsMap()
	}

	md := metadata.MD{}
	for k, v := range fields["metadata"].GetStructValue().GetFields() {
		md.Set(k, v.GetStringValue())
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

	return &grpcCall{
		address: address,
		service: service,
		method:  method,
		request: request,
		useTLS:  fields["tls"].GetBoolValue(),
		md:      md,
		timeout: timeout,
		expect:  expect,
	}, nil
}

// invoke dials call.address, resolves call.service/call.method via server
// reflection, and makes the unary call. The connection and reflection
// client are both scoped to this one call (dialed fresh, closed on
// return) rather than pooled/cached — a plugin call's target address is
// per-step config and can point at a different server on every run, so
// there's no meaningful "the same address" case to cache across calls the
// way, say, garage_plugin.go's registry list is.
func invoke(ctx context.Context, call *grpcCall) (map[string]interface{}, error) {
	var creds credentials.TransportCredentials
	if call.useTLS {
		creds = credentials.NewTLS(&tls.Config{})
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(call.address, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", call.address, err)
	}
	defer conn.Close()

	refClient := grpcreflect.NewClientAuto(ctx, conn)
	defer refClient.Reset()

	svcDesc, err := refClient.ResolveService(call.service)
	if err != nil {
		return nil, fmt.Errorf("resolving service %q via reflection on %s: %w", call.service, call.address, err)
	}
	methodDesc := svcDesc.FindMethodByName(call.method)
	if methodDesc == nil {
		return nil, fmt.Errorf("service %q on %s has no method %q", call.service, call.address, call.method)
	}
	if methodDesc.IsClientStreaming() || methodDesc.IsServerStreaming() {
		return nil, fmt.Errorf("method %s.%s is streaming; only unary methods are supported", call.service, call.method)
	}

	reqJSON, err := json.Marshal(call.request)
	if err != nil {
		return nil, fmt.Errorf("config.request could not be marshaled to JSON: %w", err)
	}
	reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
	if err := reqMsg.UnmarshalJSON(reqJSON); err != nil {
		return nil, fmt.Errorf("config.request does not match %s's input type %s: %w", call.method, methodDesc.GetInputType().GetFullyQualifiedName(), err)
	}

	callCtx := ctx
	if len(call.md) > 0 {
		callCtx = metadata.NewOutgoingContext(ctx, call.md)
	}

	stub := grpcdynamic.NewStub(conn)
	respMsg, err := stub.InvokeRpc(callCtx, methodDesc, reqMsg)

	respMap := map[string]interface{}{}
	if dynResp, ok := respMsg.(*dynamic.Message); ok && dynResp != nil {
		respJSON, marshalErr := dynResp.MarshalJSON()
		if marshalErr == nil {
			_ = json.Unmarshal(respJSON, &respMap)
		}
	}

	return respMap, err
}

// evaluateExpect checks expect.status and expect.response against the
// call's outcome — the grpc-call equivalent of
// services/tooling/http-call/rpc.go's evaluateExpect, adapted for a grpc
// status code and a decoded response message instead of an HTTP status
// and JSON body.
func evaluateExpect(expect *structpb.Struct, code interface{ String() string }, respMap map[string]interface{}) []string {
	var reasons []string

	if statusField, ok := expect.GetFields()["status"]; ok {
		want := statusField.GetStringValue()
		got := code.String()
		if want == "" {
			want = "OK"
		}
		if !strings.EqualFold(want, got) {
			reasons = append(reasons, fmt.Sprintf("expect.status: %s, got %s", want, got))
		}
	}

	if responseField, ok := expect.GetFields()["response"]; ok {
		reasons = append(reasons, evaluateBodyExpect("response", responseField.GetStructValue(), respMap)...)
	}

	return reasons
}

// evaluateBodyExpect recursively walks an expect.response struct against
// the decoded response message — the same duplicated leaf-assertion logic
// as services/tooling/http-call/rpc.go's function of the same name (which
// itself duplicates services/tooling/bench/grpc_call.go's); see
// http-call's file comment for why it's copied rather than shared.
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
	return string(actualJSON) == string(wantJSON)
}
