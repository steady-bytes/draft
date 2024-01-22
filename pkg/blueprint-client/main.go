package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	rfv1 "github.com/steady-bytes/draft/api/consensus/raft/v1"
	rfv1Cnt "github.com/steady-bytes/draft/api/consensus/raft/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/registry/key_value/v1"
	kvv1Cnt "github.com/steady-bytes/draft/api/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	CMD            = "CMD"
	SERVER_ADDRESS = "http://localhost:2221"
)

var (
	NODE_ADDRESSES = map[string]string{
		"node_1": "http://localhost:2221",
		"node_2": "http://localhost:2222",
		"node_3": "http://localhost:2221",
	}
)

func main() {
	cmd := os.Getenv(CMD)
	if cmd == "" {
		fmt.Println("enter a command")
		return
	}

	if cmd == "set_value" {
		setValue()
	}

	if cmd == "get_value" {
		getValue()
	}

	if cmd == "make_cluster" {
		makeCluster()
	}

	if cmd == "load_test_key_value" {
		loadTestKeyValue()
	}
}

// setValue - A test of the key/value interface
func setValue() {
	val, err := anypb.New(&kvv1.Value{
		Data: "how will the any pb work?",
	})
	if err != nil {
		panic("failed to create the `value` struct")
	}

	req := connect.NewRequest(&kvv1.SetRequest{
		Key:   "test",
		Value: val,
	})

	client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)
	res, err := client.Set(context.Background(), req)
	if err != nil {
		panic("set failed")
	}

	fmt.Println("response: ", res)
}

func getValue() {

	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		panic("failed to create the `value` struct")
	}

	req := connect.NewRequest(&kvv1.GetRequest{
		Key:   "22fc0a9f-99f5-476a-8f93-235737915142",
		Value: val,
	})

	for _, val := range NODE_ADDRESSES {
		client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, val)
		res, err := client.Get(context.Background(), req)
		if err != nil {
			panic("set failed")
		}

		fmt.Println("response: ", res.Msg.GetValue())
	}
}

func makeCluster() {
	raftClient := rfv1Cnt.NewRaftServiceClient(http.DefaultClient, SERVER_ADDRESS)

	req := connect.NewRequest(
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
}

func loadTestKeyValue() {
	keyValClient := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)
	keys := make([]string, 0)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		k := uuid.NewString()
		v := uuid.NewString()

		go func() {
			defer wg.Done()

			// call key/val client
			var (
				key = k
				val = &kvv1.Value{
					Data: v,
				}
			)

			v, err := anypb.New(val)
			if err != nil {
				fmt.Println("error converting to anypb")
			}

			req := connect.NewRequest(&kvv1.SetRequest{
				Key:   key,
				Value: v,
			})

			k, err := keyValClient.Set(context.Background(), req)
			if err != nil {
				fmt.Println("failed to save key")
			}

			keys = append(keys, k.Msg.GetKey())

		}()
	}

	wg.Wait()

	for _, i := range keys {
		val, err := anypb.New(&kvv1.Value{})
		if err != nil {
			fmt.Println("error")
		}

		req4 := connect.NewRequest(&kvv1.GetRequest{
			Key:   i,
			Value: val,
		})

		getRes, err := keyValClient.Get(context.Background(), req4)
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("res:  ", i, getRes.Msg.GetValue().GetValue())
		m, err := anypb.UnmarshalNew(getRes.Msg.GetValue(), proto.UnmarshalOptions{})
		if err != nil {
			fmt.Println("unmarshal failed")
		}

		fmt.Println("stored message: ", m)

		v2 := &kvv1.Value{}
		if err := anypb.UnmarshalTo(getRes.Msg.GetValue(), v2, proto.UnmarshalOptions{}); err != nil {
			fmt.Println("failed to unmarshal")
		}

		fmt.Println("store message deserialized/marshaled into specific type: ", v2.GetData())
	}
}
