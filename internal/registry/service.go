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

func (s *service) InitiateHandshake(ctx context.Context, req *api.RequestHandshake) (*api.Handshake, error) {
	// unpack request payload
	payload := req.GetPayload()

	// validate
	if err := payload.Validate(); err != nil {
		msg := fmt.Sprintf("payload is not valid %s", err)
		return nil, errors.New(msg)
	}

	// store in db
	_, err := api.DefaultCreateProcess(ctx, payload, s.DB)
	if err != nil {
		msg := fmt.Sprintf("payload was not stored %s", err)
		return nil, errors.New(msg)
	}

	// send event

	return nil, errors.New("implement me")
}

func (s *service) Connect(stream api.Registry_ConnectServer) error {
	return errors.New("implement me")
}

func (s *service) Disconnect(ctx context.Context, req *api.DisconnectRequest) (*api.Disconnected, error) {
	return nil, errors.New("implement me")
}

func (s *service) Monitor(req *api.MonitorRequest, stream api.Registry_MonitorServer) error {
	return errors.New("implement me")
}

func (s *service) QuerySystemJournal(ctx context.Context, req *api.JournalQueryRequest) (*api.JournalQueryResponse, error) {
	return nil, errors.New("implement me")
}
