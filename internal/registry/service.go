package registry

import (
	"context"
	"errors"
	"fmt"

	api "github.com/steady-bytes/draft/api/gen/go"

	"github.com/jinzhu/gorm"
)

type service struct {
	api.RegistryServer
	DB *gorm.DB
}

func NewService() *service {
	return &service{}
}

// RPC INTERFACE IMPLEMENTATION

func (s *service) Join(ctx context.Context, join *api.JoinRequest) (*api.JoinResponse, error) {
	// unpack request payload
	payload := join.GetPayload()

	// validate
	if err := payload.Validate(); err != nil {
		msg := fmt.Sprintf("payload is not valid %s", err)
		return nil, errors.New(msg)
	}

	// store in db
	res, err := api.DefaultCreateProcess(ctx, payload, s.DB)
	if err != nil {
		msg := fmt.Sprintf("payload was not stored %s", err)
		return nil, errors.New(msg)
	}

	// send event

	// return response
	return &api.JoinResponse{
		Result: res,
	}, nil
}

func (s *service) Leave(ctx context.Context, leave *api.LeaveRequest) (*api.LeaveResponse, error) {
	return nil, errors.New("implement me")
}
