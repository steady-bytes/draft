package client

import (
	"errors"
	"fmt"

	sdv1 "github.com/steady-bytes/draft/api/registry/service_discovery/v1/v1connect"
	"google.golang.org/grpc"
)

type (
	BlueprintClient interface {
		Initialize()
		Synchronize()
		Finalize()
		ReportHealth()
		Query()
	}

	blueprintClient struct {
		rpc sdv1.ServiceDiscoveryServiceClient
	}
)

var (
	ErrFailedRPCConnect = errors.New("failed to connect to blueprint")
)

func NewClient() BlueprintClient {
	entrypoint := "localhost:51000"

	conn, err := grpc.Dial(entrypoint, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("[%s] Dial failed: %v", entrypoint, err)
		panic(ErrFailedRPCConnect)
	}

	blueprintRpcClient := sdv1.NewServiceDiscoveryServiceClient(conn, entrypoint)

	return &blueprintClient{
		rpc: conn,
	}
}

func (c *blueprintClient) Initialize() {
	// do something
}

func (c *blueprintClient) Synchronize() {
	// do something
}

func (c *blueprintClient) Finalize() {
	// do something
}

func (c *blueprintClient) ReportHealth() {
	// do something
}

func (c *blueprintClient) Query() {
	// do something
}
