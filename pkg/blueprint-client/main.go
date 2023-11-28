package main

import (
	"context"
	"fmt"
	"log"
	"time"

	rfv1 "github.com/steady-bytes/draft/api/gen/go/consensus/raft/v1"
	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_val/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	fmt.Println("test blueprint")

	// create grpc client
	serverAddress := "localhost:2221"

	conn, err := grpc.Dial(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	raftClient := rfv1.NewRaftServiceClient(conn)
	keyValClient := kvv1.NewKeyValueServiceClient(conn)

	_, err = raftClient.Join(context.Background(), &rfv1.JoinRequest{
		NodeId:      "node_2",
		RaftAddress: "localhost:1112",
	})
	if err != nil {
		fmt.Println("failed to connect to leader")
	}

	_, err = raftClient.Join(context.Background(), &rfv1.JoinRequest{
		NodeId:      "node_3",
		RaftAddress: "localhost:1113",
	})
	if err != nil {
		fmt.Println("failed to connect to leader")
	}

	time.Sleep(1 * time.Second)

	// call key/val client
	res, err := keyValClient.Set(context.Background(), &kvv1.SetRequest{
		Key:   "test",
		Value: "test value",
	})
	if err != nil {
		fmt.Println("failed to save key")
	}

	fmt.Println("res: ", res)
}
