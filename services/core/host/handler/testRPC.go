package handler

import (
	"context"
	"errors"

	c "github.com/steady-bytes/draft/services/host/controller"
	"google.golang.org/grpc"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	hwv1 "github.com/steady-bytes/draft/api/go/test/hello_world/v1"
)

type (
	TestRPCHandler interface {
		draft.RPCRegistrar
	}

	testRPCHandler struct {
		hwv1.UnimplementedHelloWorldServer
		testCtrl c.TestController
	}
)

func NewTestRPCHandler() TestRPCHandler {
	return &testRPCHandler{}
}

func (r *testRPCHandler) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	hwv1.RegisterHelloWorldServer(server, r)

	return server
}

func (h *testRPCHandler) Hello(ctx context.Context, req *hwv1.HelloRequest) (*hwv1.HelloResponse, error) {
	return nil, errors.New("please implement me")
}
