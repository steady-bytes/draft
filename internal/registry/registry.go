package registry

import (
	"errors"
	"fmt"

	api "github.com/steady-bytes/draft/api/gen/go"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime"
	"google.golang.org/grpc"

	"github.com/jinzhu/gorm"
)

// The registry is a persistent service storing metadata, and process information in the system
// so that other services may read the dynamic configuration of the entier system.
//
// Because of this, an rpc interface, event_store client, and storage facility is needed so the `AggregatePluginRegistrar`
// can be used as it's base.
func NewPlugin() draft.DefaultPluginRegistrar {
	return &registry{
		service: NewService(),
	}
}

type registry struct {
	*draft.DefaultRuntimeBuilder
	service *service
}

func (r *registry) RegisterDB(db interface{}) error {
	if db == nil {
		return errors.New("db interface is nil")
	}

	if db, ok := db.(*gorm.DB); ok {
		fmt.Println("migrate process")
		db = db.AutoMigrate(&api.ProcessORM{})

		fmt.Println("migrate process metadata")
		db = db.AutoMigrate(&api.MetadataORM{})

		r.service.DB = db
	}

	return nil
}

// Implement the `draft.RpcPluginRegistrar` interface because the `EventStore`
// contains a `Create` event for external clients like `web-app`'s, `mobile` app's
// and native desktop applications to create and event known to the whole system
// of services.
func (r *registry) IsRpc() bool {
	return true
}

func (r *registry) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	api.RegisterRegistryServer(server, r.service)

	return server
}
