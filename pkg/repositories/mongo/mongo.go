package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/steady-bytes/draft/pkg/chassis"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type (
	Repository interface {
		chassis.Repository
		Client() *mongo.Client
	}
	repository struct {
		client    *mongo.Client
		configKey string
	}
)

// New instantiates a new repository. A call to Open is required before use.
// The configKey parameter dictates which key in the configuration will be read during
// initialization. Default: "repositories.mongo"
func New(configKey string) Repository {
	if configKey == "" {
		configKey = "repositories.mongo"
	}
	return &repository{
		configKey: configKey,
	}
}

func (r *repository) Client() *mongo.Client {
	return r.client
}

func (r *repository) Open(ctx context.Context, config chassis.Config) error {
	// create new client from address
	url := config.GetString(fmt.Sprintf("%s.url", r.configKey))
	client, err := mongo.NewClient(options.Client().ApplyURI(url))
	if err != nil {
		return fmt.Errorf("failed to create mongo database client with error: %s", err.Error())
	}
	r.client = client

	// connect to database with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to mongo database with error: %s", err.Error())
	}

	// since client.Connect() does not verify the connection, ping the database before returning
	err = r.Ping(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (r *repository) Close(ctx context.Context) error {
	if err := r.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect mongo client with error: %s", err.Error())
	}
	return nil
}

func (r *repository) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping database with error: %s", err.Error())
	}
	return nil
}
