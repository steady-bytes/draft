package bun

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type (
	Repository interface {
		chassis.Repository
		Client() *bun.DB
	}
	repository struct {
		client    *bun.DB
		configKey string
	}
)

// New instantiates a new repository. A call to Open is required before use.
// The configKey parameter dictates which key in the configuration will be read during
// initialization. Default: "repositories.postgres"
func New(configKey string) Repository {
	if configKey == "" {
		configKey = "repositories.postgres"
	}
	return &repository{
		configKey: configKey,
	}
}

func (r *repository) Client() *bun.DB {
	return r.client
}

func (r *repository) Open(ctx context.Context, config chassis.Config) error {
	url := config.GetString(fmt.Sprintf("%s.url", r.configKey))
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(url)))
	client := bun.NewDB(sqldb, pgdialect.New())
	r.client = client
	return nil
}

func (r *repository) Close(ctx context.Context) error {
	err := r.client.Close()
	if err != nil {
		return fmt.Errorf("failed to close the db connection for disconnect")
	}
	return nil
}

func (r *repository) Ping(ctx context.Context) error {
	err := r.client.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping the db")
	}
	return nil
}
