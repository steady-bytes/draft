package broker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"
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
	value := &kvv1.Value{
		Data: "test message",
	}

	a, _ := anypb.New(value)

	fmt.Println("value message:", value)

	attrs := make(map[string]*acv1.CloudEvent_CloudEventAttributeValue)

	attrs["isDurable"] = &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeBoolean{
			CeBoolean: true,
		},
	}

	msg := &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      string(value.ProtoReflect().Descriptor().FullName()),
		SpecVersion: "v1",
		Type:        string(value.ProtoReflect().Descriptor().Name()),
		Attributes:  attrs,
		Data: &acv1.CloudEvent_ProtoData{
			ProtoData: a,
		},
	}

	req := connect.NewRequest(&acv1.ConsumeRequest{
		Message: msg,
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
