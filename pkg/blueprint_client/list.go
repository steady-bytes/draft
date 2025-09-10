package bpc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

// list retrieves all items of type T from the key-value store.
// It's understood that the response of this is not limited in size
// so use with caution in production environments.
// Blueprint needs some additional logic to handle pagination or limits.
// I don't think this can actually be improved without Blueprint having
// indexing and pagination capabilities.
func List[T protoreflect.ProtoMessage](ctx context.Context, client kvv1Connect.KeyValueServiceClient, factory func() T) ([]T, error) {
	val, err := anypb.New(*new(T))
	if err != nil {
		return nil, err
	}

	res, err := client.List(ctx, connect.NewRequest(&kvv1.ListRequest{
		Value: val,
	}))
	if err != nil {
		return nil, err
	}

	if res.Msg == nil || len(res.Msg.GetValues()) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var items []T
	for _, v := range res.Msg.GetValues() {
		item := factory()
		if err := v.UnmarshalTo(item); err != nil {
			return nil, fmt.Errorf("failed to unmarshal item: %v", err)
		}
		items = append(items, item)
	}

	return items, nil
}
