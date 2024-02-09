package clickhouse

import (
	"context"
	"fmt"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type (
	Repository interface {
		chassis.Repository
		Client() clickhouse.Conn
	}
	repository struct {
		client    clickhouse.Conn
		configKey string
	}
)

// New instantiates a new repository. A call to Open is required before use.
// The configKey parameter dictates which key in the configuration will be read during
// initialization. Default: "repositories.clickhouse"
func New(configKey string) Repository {
	if configKey == "" {
		configKey = "repositories.clickhouse"
	}
	return &repository{
		configKey: configKey,
	}
}

func (r *repository) Client() clickhouse.Conn {
	return r.client
}

func (r *repository) Open(ctx context.Context, config chassis.Config) error {
	auth := &clickhouse.Auth{}
	err := config.UnmarshalKey(fmt.Sprintf("%s.auth", r.configKey), auth)
	if err != nil {
		return err
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: config.GetStringSlice(fmt.Sprintf("%s.addresses", r.configKey)),
		Auth: *auth,
	})
	if err != nil {
		return err
	}
	r.client = conn
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
	err := r.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping the db")
	}
	return nil
}
