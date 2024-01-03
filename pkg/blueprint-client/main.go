package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	cnt "connectrpc.com/connect"
	"github.com/google/uuid"

	rfv1 "github.com/steady-bytes/draft/api/gen/go/consensus/raft/v1"
	kvv1 "github.com/steady-bytes/draft/api/gen/go/registry/key_value/v1"
	sdv1 "github.com/steady-bytes/draft/api/gen/go/registry/service_discovery/v1"

	rfv1Cnt "github.com/steady-bytes/draft/api/go/consensus/raft/v1/v1connect"
	kvv1Cnt "github.com/steady-bytes/draft/api/go/registry/key_value/v1/v1connect"
	sdv1Cnt "github.com/steady-bytes/draft/api/go/registry/service_discovery/v1/v1connect"
)

const (
	CMD            = "CMD"
	SERVER_ADDRESS = "http://localhost:2221"
)

func main() {
	cmd := os.Getenv(CMD)
	if cmd == "" {
		fmt.Println("enter a command")
		return
	}

	if cmd == "register" {
		registerBlueprintNodes()
	}

	if cmd == "init" {
		initService()
	}
}

func initService() {
	client := sdv1Cnt.NewServiceDiscoveryServiceClient(http.DefaultClient, SERVER_ADDRESS)
	req := cnt.NewRequest(&sdv1.InitRequest{
		Name:  "test-registry",
		Nonce: "BLUEPRINT",
	})
	res, err := client.Init(context.Background(), req)
	if err != nil {
		fmt.Println("failed to init a process")
	}

	fmt.Println(res)
}

func registerBlueprintNodes() {
	fmt.Println("test blueprint")

	raftClient := rfv1Cnt.NewRaftServiceClient(http.DefaultClient, SERVER_ADDRESS)
	keyValClient := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)

	req := cnt.NewRequest(
		&rfv1.JoinRequest{
			NodeId:      "node_2",
			RaftAddress: "localhost:1112",
		})
	_, err := raftClient.Join(
		context.Background(), req)
	if err != nil {
		fmt.Println("failed to connect to leader")
	}

	req.Msg.NodeId = "node_3"
	req.Msg.RaftAddress = "localhost:1113"
	_, err = raftClient.Join(context.Background(), req)
	if err != nil {
		fmt.Println("failed to connect to leader")
	}

	time.Sleep(1 * time.Second)

	// call key/val client
	var (
		key = "test"
		val = "test value"
	)

	req2 := cnt.NewRequest(&kvv1.SetRequest{
		Key:   key,
		Value: val,
	})

	setRes, err := keyValClient.Set(context.Background(), req2)
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

			req3 := cnt.NewRequest(&kvv1.SetRequest{
				Key:   k,
				Value: v,
			})
			setRes, err := keyValClient.Set(context.Background(), req3)
			if err != nil {
				fmt.Println("failed to save key")
			}

			fmt.Println(setRes)

			keys = append(keys, k)

		}()
	}

	wg.Wait()

	for _, i := range keys {
		req4 := cnt.NewRequest(&kvv1.GetRequest{
			Key:    i,
			Filter: kvv1.GetFilter_STRING_GET_FILTER,
		})
		getRes, err := keyValClient.Get(context.Background(), req4)
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("res:  ", i, getRes.Msg.GetAsString())
	}
}
