package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
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
	var (
		key = "test"
		val = "test value"
	)
	setRes, err := keyValClient.Set(context.Background(), &kvv1.SetRequest{
		Key:   key,
		Value: val,
	})
	if err != nil {
		fmt.Println("failed to save key")
	}

	fmt.Println("res: ", setRes)

	keys := make([]string, 0)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		k := uuid.NewString()
		v := uuid.NewString()

		go func() {
			defer wg.Done()

			setRes, err := keyValClient.Set(context.Background(), &kvv1.SetRequest{
				Key:   k,
				Value: v,
			})
			if err != nil {
				fmt.Println("failed to save key")
			}

			fmt.Println(setRes)

			keys = append(keys, k)

		}()
	}

	wg.Wait()

	for _, i := range keys {
		getRes, err := keyValClient.Get(context.Background(), &kvv1.GetRequest{
			Key:    i,
			Filter: kvv1.GetFilter_STRING_GET_FILTER,
		})
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("res:  ", i, getRes.GetAsString())
	}
}
