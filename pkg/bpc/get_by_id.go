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

func GetById[T protoreflect.ProtoMessage](
	ctx context.Context,
	client kvv1Connect.KeyValueServiceClient,
	id string,
	factory func() T) (T, error) {
	if id == "" {
		return factory(), fmt.Errorf("ID cannot be empty")
	}

	t := factory()

	val, err := anypb.New(*new(T))
	if err != nil {
		return t, err
	}

	res, err := client.Get(ctx, connect.NewRequest(&kvv1.GetRequest{
		Key:   id,
		Value: val,
	}))
	if err != nil {
		return t, err
	}

	if res.Msg == nil || res.Msg.GetValue() == nil {
		return t, fmt.Errorf("item with ID %s not found", id)
	}

	if err := res.Msg.GetValue().UnmarshalTo(t); err != nil {
		return t, fmt.Errorf("failed to unmarshal item: %v", err)
	}

	return t, nil
}
