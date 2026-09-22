// store.go: thin generic helpers over Blueprint's KeyValueService -- Lineman's
// only datastore (see docs/architecture/lineman-implementation-plan.md's
// "Blueprint storage" section). Each entity type is registered with
// Blueprint's type registry (see main.go's WithRegisteredType calls) so it
// renders decoded, not as opaque bytes, in Blueprint's own Key/Value browser.
package main

import (
	"context"
	"fmt"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func kvSet(ctx context.Context, client kvv1Connect.KeyValueServiceClient, key string, msg proto.Message) error {
	val, err := anypb.New(msg)
	if err != nil {
		return fmt.Errorf("failed to wrap %T for storage: %w", msg, err)
	}
	_, err = client.Set(ctx, connect.NewRequest(&kvv1.SetRequest{Key: key, Value: val}))
	if err != nil {
		return fmt.Errorf("failed to store %T %q: %w", msg, key, err)
	}
	return nil
}

func kvGet[T proto.Message](ctx context.Context, client kvv1Connect.KeyValueServiceClient, key string, newT func() T) (T, error) {
	var zero T
	witness, err := anypb.New(newT())
	if err != nil {
		return zero, fmt.Errorf("failed to build lookup witness: %w", err)
	}
	resp, err := client.Get(ctx, connect.NewRequest(&kvv1.GetRequest{Key: key, Value: witness}))
	if err != nil {
		return zero, fmt.Errorf("not found: %w", err)
	}
	out := newT()
	if err := resp.Msg.GetValue().UnmarshalTo(out); err != nil {
		return zero, fmt.Errorf("failed to decode stored value: %w", err)
	}
	return out, nil
}

func kvList[T proto.Message](ctx context.Context, client kvv1Connect.KeyValueServiceClient, newT func() T) (map[string]T, error) {
	witness, err := anypb.New(newT())
	if err != nil {
		return nil, fmt.Errorf("failed to build list witness: %w", err)
	}
	resp, err := client.List(ctx, connect.NewRequest(&kvv1.ListRequest{Value: witness}))
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	out := make(map[string]T, len(resp.Msg.GetValues()))
	for k, v := range resp.Msg.GetValues() {
		item := newT()
		if err := v.UnmarshalTo(item); err != nil {
			return nil, fmt.Errorf("failed to decode listed value %q: %w", k, err)
		}
		out[k] = item
	}
	return out, nil
}

func kvDelete(ctx context.Context, client kvv1Connect.KeyValueServiceClient, key string, msg proto.Message) error {
	witness, err := anypb.New(msg)
	if err != nil {
		return fmt.Errorf("failed to build delete witness: %w", err)
	}
	_, err = client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{Key: key, Value: witness}))
	if err != nil {
		return fmt.Errorf("failed to delete %q: %w", key, err)
	}
	return nil
}
