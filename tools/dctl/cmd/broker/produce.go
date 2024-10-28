package broker

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"
)

func Produce(cmd *cobra.Command, args []string) error {
	fmt.Println(args)

	// connect to running catalyst process
	client := v1connect.NewProducerClient(http.DefaultClient, "http://0.0.0.0:3331", connect.WithGRPC())

	ctx := context.Background()

	streams := client.Produce(ctx)

	req := connect.NewRequest(&acv1.ProduceRequest{
		Message: &acv1.Message{
			Domain: "test",
			Kind:   &anypb.Any{},
		},
	})

	if err := streams.Send(req.Msg); err != nil {
		return err
	}

	// close connection
	// streams.CloseRequest()

	res, err := streams.Receive()
	if err != nil {
		fmt.Println("error: ", err)
	}

	fmt.Println(res)

	return nil
}
