package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	rfv1 "github.com/steady-bytes/draft/api/core/consensus/raft/v1"
	rfv1Cnt "github.com/steady-bytes/draft/api/core/consensus/raft/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Cnt "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	sdv1Cnt "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"
	"golang.org/x/net/http2"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	CMD = "CMD"
	// Change this to port 2222 or 2223 to test out set forwarding to leader
	SERVER_ADDRESS = "http://localhost:2221"
)

var (
	NODE_ADDRESSES = map[string]string{
		"node_1": "http://localhost:2221",
		"node_2": "http://localhost:2222",
		"node_3": "http://localhost:2223",
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

	if cmd == "cluster_stats" {
		clusterStats()
	}

	if cmd == "load_test_key_value" {
		loadTestKeyValue()
	}

	if cmd == "list_all" {
		listAll()
	}

	if cmd == "init_process" {
		initializeProcess()
	}

	if cmd == "synchronize" {
		synchronize()
	}
}

func synchronize() {
	name := "test-init-2"

	initReq := connect.NewRequest(&sdv1.InitializeRequest{
		Name:  name,
		Nonce: "BLUEPRINT",
	})

	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				// If you're also using this client for non-h2c traffic, you may want
				// to delegate to tls.Dial if the network isn't TCP or the addr isn't
				// in an allowlist.
				return net.Dial(network, addr)
			},
		},
	}

	sdClient := sdv1Cnt.NewServiceDiscoveryServiceClient(httpClient, SERVER_ADDRESS)
	initRes, err := sdClient.Initialize(context.Background(), initReq)
	if err != nil {
		panic("failed to Initialize the process")
	}

	syncReq := connect.NewRequest(&sdv1.ClientDetails{
		Pid:          initRes.Msg.ProcessIdentity.GetPid(),
		RunningState: sdv1.ProcessRunningState_PROCESS_RUNNING,
		HealthState:  sdv1.ProcessHealthState_PROCESS_HEALTHY,
		ProcessKind:  sdv1.ProcessKind_SERVER_PROCESS,
		Token:        initRes.Msg.ProcessIdentity.Token.GetJwt(),
		Location:     &sdv1.GeoPoint{},
		Metadata:     []*sdv1.Metadata{},
	})
	stream := sdClient.Synchronize(context.Background())
	waitc := make(chan struct{})

	// fire a receive stream
	go func() {
		for {
			in, err := stream.Receive()
			if err == io.EOF {
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("failed %v", err)
			}
			log.Printf("go message %s", in.GetNodes())
		}
	}()

	// send messages to blueprint
	r := 0
	for r != 100 {
		fmt.Println("send detail packet", initRes.Msg.ProcessIdentity.GetPid())

		if err := stream.Send(syncReq.Msg); err != nil {
			fmt.Println(err)
		}

		r++

		time.Sleep(1 * time.Second)
	}

	if err := stream.CloseRequest(); err != nil {
		log.Print(err)
	}

	<-waitc
}

func initializeProcess() {
	name := "test-init-1"

	initReq := connect.NewRequest(&sdv1.InitializeRequest{
		Name:  name,
		Nonce: "BLUEPRINT",
	})

	sdClient := sdv1Cnt.NewServiceDiscoveryServiceClient(http.DefaultClient, SERVER_ADDRESS)
	initRes, err := sdClient.Initialize(context.Background(), initReq)
	if err != nil {
		panic("failed to Initialize the process")
	}

	fmt.Println("init response: ", initRes.Msg)

	val, err := anypb.New(&sdv1.Process{})
	if err != nil {
		panic("failed to create the `value` struct")
	}

	getReq := connect.NewRequest(&kvv1.GetRequest{
		Key:   initRes.Msg.ProcessIdentity.GetPid(),
		Value: val,
	})

	kvClient := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)
	getRes, err := kvClient.Get(context.Background(), getReq)
	if err != nil {
		fmt.Println("get failed: ", err)
		return
	}

	fmt.Println("response: ", getRes.Msg.GetValue())
}

func listAll() {
	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		panic("failed to create the `value` struct")
	}

	req := connect.NewRequest(&kvv1.ListRequest{
		Value: val,
	})

	// responses := []map[string]*anypb.Any{}

	client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)
	res, err := client.List(context.Background(), req)
	if err != nil {
		panic("list failed")
	}

	fmt.Println(res)

	// if len(responses) != 3 {
	// 	panic("fail list")
	// }

	// for k, val := range responses[0] {
	// 	val1, ok := responses[1][k]
	// 	if !ok {
	// 		panic("key not found in first node")
	// 	}

	// 	val2, ok := responses[2][k]
	// 	if !ok {
	// 		panic("key not found in third node")
	// 	}

	// 	if string(val.Value) != string(val1.Value) && string(val.Value) != string(val2.Value) {
	// 		panic("values for keys are not equal")
	// 	}
	// }
}

// setValue - A test of the key/value interface
func setValue() {
	val, err := anypb.New(&kvv1.Value{
		Data: uuid.New().String(),
	})
	if err != nil {
		panic("failed to create the `value` struct")
	}

	req := connect.NewRequest(&kvv1.SetRequest{
		Key:   "test",
		Value: val,
	})

	fmt.Printf("attempting to set: %v\n", req)
	client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)
	res, err := client.Set(context.Background(), req)
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Println("response: ", res)
}

func getValue() {

	val, err := anypb.New(&kvv1.Value{})
	if err != nil {
		panic("failed to create the `value` struct")
	}

	req := connect.NewRequest(&kvv1.GetRequest{
		Key:   "test",
		Value: val,
	})

	for key, val := range NODE_ADDRESSES {
		fmt.Printf("reading from: %s\n", key)
		client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, val)
		res, err := client.Get(context.Background(), req)
		if err != nil {
			fmt.Printf("error: %s\n", err.Error())
		} else {
			fmt.Println("response: ", res.Msg.GetValue())
		}
	}
}

func makeCluster() {
	raftClient := rfv1Cnt.NewRaftServiceClient(http.DefaultClient, SERVER_ADDRESS)

	req := connect.NewRequest(
		&rfv1.JoinRequest{
			NodeId:      "node_2",
			RaftAddress: "localhost:1112",
		})
	_, err := raftClient.Join(context.Background(), req)
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

func clusterStats() {
	raftClient := rfv1Cnt.NewRaftServiceClient(http.DefaultClient, SERVER_ADDRESS)

	req := connect.NewRequest(&rfv1.StatsRequest{})
	res, err := raftClient.Stats(context.Background(), req)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("res: ", res)
}

const (
	LOAD_COUNT = 1000
)

func loadTestKeyValue() {
	keyValClient := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, SERVER_ADDRESS)
	keys := make([]string, 0)

	var wg sync.WaitGroup

	for i := 0; i < LOAD_COUNT; i++ {
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
				return
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
