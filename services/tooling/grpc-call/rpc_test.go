// Tests exercise the real reflection + dynamic-invocation path end to
// end, not a mocked resolver: rather than inventing a synthetic test-only
// .proto (this repo's convention keeps all protos under api/, generated
// via buf — see /Users/andrew/Projects/steady-bytes/CLAUDE.md), these
// tests stand up a real *grpc.Server exposing this repo's own already-
// generated tooling.step_executor.v1.StepExecutor service (reusing its
// real, already-registered file descriptor — protoc-gen-go registers a
// full FileDescriptorProto, service definitions included, in the global
// proto registry regardless of whether grpc-go bindings were generated
// for it) with reflection enabled, exactly like a real external gRPC
// server this plugin is meant to call. A hand-built grpc.ServiceDesc
// wires a small in-test handler up to that same real ServiceName/Metadata
// pair, so grpcreflect resolves it exactly as it would resolve any real
// service.
package main

import (
	"context"
	"net"
	"testing"

	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// noopLogger is a minimal chassis.Logger for tests — mirrors
// services/tooling/catalyst-consume/rpc_test.go's noopLogger.
type noopLogger struct{}

func (noopLogger) Start(chassis.Config)                         {}
func (noopLogger) SetLevel(chassis.LogLevel)                    {}
func (noopLogger) GetLevel() chassis.LogLevel                   { return chassis.InfoLevel }
func (noopLogger) Wrap(err error) error                         { return err }
func (l noopLogger) WithError(error) chassis.Logger             { return l }
func (l noopLogger) WithContext(context.Context) chassis.Logger { return l }
func (l noopLogger) WithField(string, any) chassis.Logger       { return l }
func (l noopLogger) WithFields(chassis.Fields) chassis.Logger   { return l }
func (l noopLogger) WithCallDepth(int) chassis.Logger           { return l }
func (noopLogger) Trace(string)                                 {}
func (noopLogger) Debug(string)                                 {}
func (noopLogger) Debugf(string, ...any)                        {}
func (noopLogger) Info(string)                                  {}
func (noopLogger) Infof(string, ...any)                         {}
func (noopLogger) Warn(string)                                  {}
func (noopLogger) Warnf(string, ...any)                         {}
func (noopLogger) Error(string)                                 {}
func (noopLogger) Errorf(string, ...any)                        {}
func (noopLogger) WrappedError(error, string)                   {}
func (noopLogger) Fatal(string)                                 {}
func (noopLogger) Panic(string)                                 {}

var _ chassis.Logger = noopLogger{}

func stepConfig(t *testing.T, fields map[string]interface{}) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("failed to build config struct: %v", err)
	}
	return s
}

// testStepExecutorServer backs the fake service's single Execute method —
// respond is set per test to control the returned StepResponse/error.
type testStepExecutorServer struct {
	respond func(*stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error)
}

func (s *testStepExecutorServer) execute(ctx context.Context, req *stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error) {
	return s.respond(req)
}

// executeHandler adapts testStepExecutorServer.execute to grpc.MethodDesc's
// Handler signature, decoding the real stepexecutorv1.StepRequest proto
// type off the wire — this is what grpc-call's dynamic client will have
// serialized from its JSON config.request via the resolved input type.
func executeHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(stepexecutorv1.StepRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*testStepExecutorServer).execute(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/tooling.step_executor.v1.StepExecutor/Execute"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*testStepExecutorServer).execute(ctx, req.(*stepexecutorv1.StepRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// testStepExecutorServiceDesc reuses the real, already-registered
// tooling.step_executor.v1.StepExecutor service name and file metadata —
// see this file's header comment for why that makes reflection resolve it
// exactly as it would a real generated grpc-go service.
var testStepExecutorServiceDesc = grpc.ServiceDesc{
	ServiceName: "tooling.step_executor.v1.StepExecutor",
	HandlerType: (*any)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Execute", Handler: executeHandler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: stepexecutorv1.File_tooling_step_executor_v1_service_proto.Path(),
}

// startTestServer starts a real *grpc.Server (reflection enabled) backed
// by respond, and returns its address. Stopped automatically via
// t.Cleanup.
func startTestServer(t *testing.T, respond func(*stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error)) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	grpcServer.RegisterService(&testStepExecutorServiceDesc, &testStepExecutorServer{respond: respond})
	reflection.Register(grpcServer)

	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	return lis.Addr().String()
}

func newTestHandler() Handler {
	return NewHandler(noopLogger{})
}

func TestExecute_Success(t *testing.T) {
	addr := startTestServer(t, func(req *stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error) {
		if req.GetStepName() != "hello-step" {
			t.Errorf("server saw step_name %q, want %q", req.GetStepName(), "hello-step")
		}
		result, _ := structpb.NewStruct(map[string]interface{}{"greeting": "hello, Ada"})
		return &stepexecutorv1.StepResponse{Success: true, Result: result}, nil
	})

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "call-echo",
		Config: stepConfig(t, map[string]interface{}{
			"address": addr,
			"service": "tooling.step_executor.v1.StepExecutor",
			"method":  "Execute",
			"request": map[string]interface{}{"step_name": "hello-step"},
			"expect": map[string]interface{}{
				"status": "OK",
				"response": map[string]interface{}{
					"success": map[string]interface{}{"equals": true},
				},
			},
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true (error: %s)", resp.Msg.GetError())
	}

	response, _ := resp.Msg.GetResult().AsMap()["response"].(map[string]interface{})
	result, _ := response["result"].(map[string]interface{})
	if result["greeting"] != "hello, Ada" {
		t.Errorf("result.response.result.greeting = %v, want %q", result["greeting"], "hello, Ada")
	}
}

func TestExecute_NonOKStatusWithoutExpectFails(t *testing.T) {
	addr := startTestServer(t, func(req *stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error) {
		return nil, status.Error(codes.NotFound, "no such thing")
	})

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "call-echo",
		Config: stepConfig(t, map[string]interface{}{
			"address": addr,
			"service": "tooling.step_executor.v1.StepExecutor",
			"method":  "Execute",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a NotFound response with no expect: configured")
	}
	if resp.Msg.GetError() == "" {
		t.Error("Error is empty, want a message describing the grpc failure")
	}
}

func TestExecute_ExpectSpecificStatusCodeMatches(t *testing.T) {
	addr := startTestServer(t, func(req *stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error) {
		return nil, status.Error(codes.NotFound, "no such thing")
	})

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "call-echo",
		Config: stepConfig(t, map[string]interface{}{
			"address": addr,
			"service": "tooling.step_executor.v1.StepExecutor",
			"method":  "Execute",
			"expect":  map[string]interface{}{"status": "NotFound"},
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("Success = false, want true when expect.status: NotFound matches a real NotFound response (error: %s)", resp.Msg.GetError())
	}
}

func TestExecute_UnknownMethodFails(t *testing.T) {
	addr := startTestServer(t, func(req *stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error) {
		return &stepexecutorv1.StepResponse{Success: true}, nil
	})

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "call-echo",
		Config: stepConfig(t, map[string]interface{}{
			"address": addr,
			"service": "tooling.step_executor.v1.StepExecutor",
			"method":  "NoSuchMethod",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a method that doesn't exist on the resolved service")
	}
}

func TestExecute_UnknownServiceFails(t *testing.T) {
	addr := startTestServer(t, func(req *stepexecutorv1.StepRequest) (*stepexecutorv1.StepResponse, error) {
		return &stepexecutorv1.StepResponse{Success: true}, nil
	})

	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "call-echo",
		Config: stepConfig(t, map[string]interface{}{
			"address": addr,
			"service": "no.such.Service",
			"method":  "Execute",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for a service that reflection can't resolve")
	}
}

func TestExecute_MissingRequiredConfig(t *testing.T) {
	h := newTestHandler()

	for _, tc := range []struct {
		name   string
		config map[string]interface{}
	}{
		{"missing address", map[string]interface{}{"service": "a.B", "method": "C"}},
		{"missing service", map[string]interface{}{"address": "localhost:1", "method": "C"}},
		{"missing method", map[string]interface{}{"address": "localhost:1", "service": "a.B"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&stepexecutorv1.StepRequest{
				StepName: "bad-config",
				Config:   stepConfig(t, tc.config),
			})
			resp, err := h.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("Execute returned unexpected error: %v", err)
			}
			if resp.Msg.GetSuccess() {
				t.Fatal("Success = true, want false")
			}
		})
	}
}

func TestExecute_NilConfigIsRPCError(t *testing.T) {
	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{StepName: "call"})

	_, err := h.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("Execute returned nil error for a nil config, want a CodeInvalidArgument error")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestExecute_UnreachableAddressFails(t *testing.T) {
	h := newTestHandler()
	req := connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: "call-echo",
		Config: stepConfig(t, map[string]interface{}{
			"address": "127.0.0.1:1",
			"service": "tooling.step_executor.v1.StepExecutor",
			"method":  "Execute",
			"timeout": "1s",
		}),
	})

	resp, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("Success = true, want false for an unreachable address")
	}
}
