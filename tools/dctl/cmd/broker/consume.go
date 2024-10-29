package broker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/types/known/anypb"
)

func Consume(cmd *cobra.Command, args []string) error {
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

	client := v1connect.NewConsumerClient(httpClient, "http://0.0.0.0:3331", connect.WithGRPC())

	ctx := context.Background()

	evt := &kvv1.Value{}
	a, _ := anypb.New(evt)

	req := connect.NewRequest(&acv1.ConsumeRequest{
		Message: &acv1.Message{
			Domain: "test",
			Kind:   a,
		},
		Count: &acv1.Count{},
	})

	stream, err := client.Consume(ctx, req)
	if err != nil {
		fmt.Println("error from catalyst when attempting to start a consumer: ", err)
	}

	// do forever
	for {
		if received := stream.Receive(); received {

			if err := stream.Err(); err != nil {
				fmt.Println("stream received a non-zero error", err)
			}

			if msg := stream.Msg(); msg != nil {
				fmt.Println("stream received a msg: ", msg)
			}
		}
	}
}
