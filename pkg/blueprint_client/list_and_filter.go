package bpc

import (
	"context"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func ListAndFilter[T protoreflect.ProtoMessage](ctx context.Context, client kvv1Connect.KeyValueServiceClient, filter *kvv1.Statement, factory func() T) ([]T, error) {
	// This function can be used to list and filter items based on the provided filter.
	// For now, it just calls the list function.
	items, err := List[T](ctx, client, factory)
	if err != nil {
		return nil, err
	}

	// filter items based on the key/value pairs in the filter
	filters := filter.GetWhere().(*kvv1.Statement_KeyVal).KeyVal.GetMatch()

	// the new slice of T that will hold the filtered items
	var filtered []T

	// use proto reflect to filter items based on the the key/value pairs in the filter
	// NOTE: This is a simple filtering logic that checks if the field value matches the filter value in
	//       In the request. It's also understood this is no the most efficient way to filter items,
	//       but it serves as a starting point for filtering logic.
	for _, item := range items {
		itemValue := item.ProtoReflect()
		matched := true
		for key, value := range filters {
			field := itemValue.Descriptor().Fields().ByName(protoreflect.Name(key))
			if field == nil || itemValue.Get(field).String() != value {
				matched = false
				break
			}
		}

		if matched {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}
