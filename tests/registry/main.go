package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	api "github.com/steady-bytes/draft/api/gen/go"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	res, err := testInitateHandshake()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("hanshake: ", res)

	testConnect(res)
}

func testConnect(handshake *api.Handshake) {
	// create url
	url := fmt.Sprintf("%s:%d", "localhost", 50000)

	// create the grpc client
	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("[%s] Dial failed: %v", url, err)
		panic(err)
	}

	client := api.NewRegistryClient(conn)

	stream, err := client.ConnectProcess(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println("enter forever loop")

	for {
		status := &api.ProcessDetails{
			ProcessId:     handshake.GetProcessId(),
			RunningState:  2,
			ProcessHealth: 1,
			Token:         handshake.GetToken().GetJwt(),
			Nonce:         handshake.GetToken().GetNonce(),
		}

		fmt.Println("sending status: ", status)

		if err := stream.Send(status); err != nil {
			fmt.Println("close message: ", err)
			panic(err)
		}

		time.Sleep(5 * time.Second)

	}
}

func testInitateHandshake() (*api.Handshake, error) {
	// create url
	url := fmt.Sprintf("%s:%d", "localhost", 50000)

	// create the grpc client
	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("[%s] Dial failed: %v", url, err)
		return nil, err
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
			JoinedTime:    timestamppb.Now(),
			Version:       "1.0.0",
			RunningState:  1,
			ProcessHealth: 1,
			Token: &api.Token{
				Id:    clientID,
				Jwt:   "",
				Nonce: "",
			},
		},
	}

	// create context
	ctx := context.Background()

	// make rpc call
	res, err := client.InitiateHandshake(ctx, req)
	if err != nil {
		return nil, err
	}

	fmt.Println("handshake response: ", res)

	return res, nil
}
