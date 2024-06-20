package handler

// import (
// 	"context"
// 	"errors"
// 	"fmt"

// 	api "github.com/steady-bytes/draft/api/go"
// )

// // create the default crud implementation
// func NewRPC() *api.EventStoreDefaultServer {
// 	return &api.EventStoreDefaultServer{}
// }

// // decorate the default `Create` interface implementing custom business logic around the currently generated `Create` implementation
// func (s *service) Create(ctx context.Context, req *api.CreateEventRequest) (*api.CreateEventResponse, error) {
// 	fmt.Println("req: ", req)

// 	if err := req.Validate(); err != nil {
// 		fmt.Println("validation failed: ", err)
// 		return nil, errors.New("failed input validation")
// 	}

// 	res, err := s.rpc.Create(ctx, req)
// 	if err != nil {
// 		fmt.Println("failed to insert: ", err)
// 		return nil, err
// 	}

// 	topic := req.GetPayload().GetAggregateKind().String()
// 	if topic == "" {
// 		return nil, errors.New("failed topic length validation")
// 	}

// 	event := req.GetPayload().GetData()

// 	if err := s.msg.Publish(topic, []byte(event)); err != nil {
// 		fmt.Println("failed publish")
// 		return nil, err
// 	}

// 	return res, nil
// }
