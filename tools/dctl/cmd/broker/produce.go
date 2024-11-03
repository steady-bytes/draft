package broker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/types/known/anypb"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
)

func Produce(cmd *cobra.Command, args []string) error {
	fmt.Println(args)

	// connect to running catalyst process
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

	client := v1connect.NewProducerClient(httpClient, "http://0.0.0.0:3331", connect.WithGRPC())

	ctx := context.Background()

	streams := client.Produce(ctx)

	evt := &kvv1.Value{
		Data: "This is a test",
	}

	a, _ := anypb.New(evt)

	req := connect.NewRequest(&acv1.ProduceRequest{
		Message: &acv1.Message{
			Domain: "test",
			Kind:   a,
		},
	})

	var wg sync.WaitGroup

	for i := 1; i < 10; i++ {
		wg.Add(1)
		if err := streams.Send(req.Msg); err != nil {
			return err
		}
		fmt.Println("sent test message")
	}

	// close connection
	// streams.CloseRequest()

	res, err := streams.Receive()
	if err != nil {
		fmt.Println("error: ", err)
	}

	if res != nil {
		fmt.Println("response back from catalyst: ", res)
		wg.Done()
	}

	wg.Wait()

	return nil
}
