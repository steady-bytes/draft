package event_store

import (
	"errors"

	api "github.com/steady-bytes/draft/api/gen/go"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime"

	"github.com/jinzhu/gorm"
	nats "github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

// Implementing the `draft.Plugin` interface so it can be run by the runtime

// Constructor to build a plugin that can be used by the runtime
func NewPlugin() draft.DefaultPluginRegistrar {
	return &eventStorePlugin{
		service: NewService(),
	}
}

// eventStorePlugin is configuration for what features are enabeld in the runtime
type eventStorePlugin struct {
	*draft.DefaultRuntimeBuilder
	service *service
}

func (s *eventStorePlugin) RegisterDB(db interface{}) error {
	if db == nil {
		return errors.New("db interface is nil")
	}

	if db, ok := db.(*gorm.DB); ok {
		db = db.AutoMigrate(&api.EventORM{})
		s.service.rpc.DB = db
	}

	return nil
}

func (s *eventStorePlugin) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	api.RegisterEventStoreServer(server, s.service.rpc)

	return server
}

func (s *eventStorePlugin) RegisterBroker(broker interface{}) error {
	if broker == nil {
		return errors.New("broker connection is nil")
	}

	s.service.msg.broker = broker.(*nats.Conn)

	return nil
}
