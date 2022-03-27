package event_store

import (
	"errors"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime"

	nats "github.com/nats-io/nats.go"
	api "github.com/steady-bytes/draft/api/gen/go"

	"github.com/jinzhu/gorm"
	"google.golang.org/grpc"
)

// Implementing the `draft.Plugin` interface so it can be run by the runtime

// Constructor to build a plugin that can be used by the runtime
func NewPlugin() draft.DefaultPluginRegistrar {
	return &eventStorePlugin{
		// type flags that enabled differnt features of the eventStorePlugin
		repoType:   draft.PostgresGorm,
		brokerType: draft.Nats,
		model:      &api.EventORM{},
		service:    NewService(),
	}
}

// eventStorePlugin is configuration for what features are enabeld in the runtime
type eventStorePlugin struct {
	repoType   draft.RepoType
	brokerType draft.BrokerType
	model      *api.EventORM
	service    *service
}

// Implement the `draft.RepoPluginRegistrar` interface
// so that we can use `Postgres` to store all of the `Event`'s in a
// denormalized format.
func (s *eventStorePlugin) GetRepoType() draft.RepoType {
	return s.repoType
}

func (s *eventStorePlugin) SetModel() interface{} {
	return s.model
}

func (s *eventStorePlugin) RegisterDB(db interface{}) error {
	if db == nil {
		return errors.New("db interface is nil")
	}

	if db, ok := db.(*gorm.DB); ok {
		s.service.rpc.DB = db
	}

	return nil
}

// Implement the `draft.RpcPluginRegistrar` interface because the `EventStore`
// contains a `Create` event for external clients like `web-app`'s, `mobile` app's
// and native desktop applications to create and event known to the whole system
// of services.
func (s *eventStorePlugin) GetIsRpc() bool {
	return true
}

func (s *eventStorePlugin) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	api.RegisterEventStoreServer(server, s.service.rpc)

	return server
}

// Implement the `draft.ProducerPluginRegistrar` interface so that each `Event` can be
// routed to it's correct topic. Currently aggregates are equal to topics.
func (s *eventStorePlugin) GetBrokerType() draft.BrokerType {
	return s.brokerType
}

func (s *eventStorePlugin) RegisterPublisher(broker interface{}) error {
	if broker == nil {
		return errors.New("broker connection is nil")
	}

	s.service.pub.broker = broker.(*nats.Conn)

	return nil
}
