package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	api "github.com/steady-bytes/draft/api/gen/go"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO: change this to the testing package

func main() {
	if err := testInitateHandshake(); err != nil {
		fmt.Println(err)
	}
}

func testInitateHandshake() error {
	// create url
	url := fmt.Sprintf("%s:%d", "localhost", 50002)

	// create the grpc client
	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("[%s] Dial failed: %v", url, err)
		return err
	}

	clientID := "78f5b6e1-3096-4d40-8bdc-8061d2cc0751"

	client := api.NewRegistryClient(conn)

	req := &api.RequestHandshake{
		Payload: &api.Process{
			Id:          clientID,
			Name:        "test process",
			Group:       "groupID",
			Local:       "centralUS",
			IpAddress:   "none",
			ProcessKind: 1,
			Tags: []*api.Metadata{
				{
					Id:    uuid.NewString(),
					Key:   "test",
					Value: "test",
				},
			},
			JoinedTime: &timestamppb.Timestamp{
				Seconds: 0,
				Nanos:   0,
			},
			LeftTime: &timestamppb.Timestamp{
				Seconds: 0,
				Nanos:   0,
			},
			Version:       "1.0.0",
			RunningState:  1,
			ProcessHealth: 1,
			Token: &api.Token{
				Id:    clientID,
				Token: "",
				Nonce: "",
			},
		},
	}

	// create context
	ctx := context.Background()

	// make rpc call
	res, err := client.InitiateHandshake(ctx, req)
	if err != nil {
		return err
	}

	fmt.Println(res)

	return nil
}
