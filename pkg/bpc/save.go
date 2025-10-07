package bpc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
)

func Save[T protoreflect.ProtoMessage](
	ctx context.Context,
	client kvv1Connect.KeyValueServiceClient,
	key string,
	item T,
) (T, error) {
	if key == "" {
		// I am starting to consider that I should make a generic error type for the repository lib
		return item, fmt.Errorf("key cannot be empty")
	}

	val, err := anypb.New(item)
	if err != nil {
		return item, err
	}

	req := connect.NewRequest(&kvv1.SetRequest{
		Key:   key,
		Value: val,
	})

	if _, err := client.Set(ctx, req); err != nil {
		return item, err
	}

	return item, nil
}
