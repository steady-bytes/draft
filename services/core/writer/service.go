package writer

import (
	"context"
	"errors"

	api "github.com/steady-bytes/draft/api/go"
)

func NewService() *service {
	return &service{}
}

type service struct {
	api.WriterServer
}

func (s *service) Exec(ctx context.Context, cmd *api.Command) (*api.Output, error) {
	// check type

	// check registry?

	// permissions

	// validate

	// proxy to the correct aggregate via the details of the registry

	return nil, errors.New("implement me")
}

func (s *service) ExecSaga(ctx context.Context, cmd *api.Command) (*api.Transaction, error) {
	return nil, errors.New("implement me")
}
