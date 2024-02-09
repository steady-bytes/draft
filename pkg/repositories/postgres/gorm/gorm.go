package gorm

import (
	"context"
	"fmt"

	"github.com/steady-bytes/draft/pkg/chassis"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type (
	Repository interface {
		chassis.Repository
		Client() *gorm.DB
	}
	repository struct {
		client    *gorm.DB
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

func (r *repository) Client() *gorm.DB {
	return r.client
}

func (r *repository) Open(ctx context.Context, config chassis.Config) error {
	url := config.GetString(fmt.Sprintf("%s.url", r.configKey))
	client, err := gorm.Open(postgres.Open(url), &gorm.Config{
		FullSaveAssociations: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to sql db")
	}
	r.client = client
	return nil
}

func (r *repository) Close(ctx context.Context) error {
	db, err := r.client.DB()
	if err != nil {
		return fmt.Errorf("failed to get the db connection for disconnect")
	}
	err = db.Close()
	if err != nil {
		return fmt.Errorf("failed to close the db connection for disconnect")
	}
	return nil
}

func (r *repository) Ping(ctx context.Context) error {
	db, err := r.client.DB()
	if err != nil {
		return fmt.Errorf("failed to get the db connection for ping")
	}
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("failed to ping sql db")
	}
	return nil
}
