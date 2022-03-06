package queirer

import (
	"errors"

	"api"
	"commet"

	"github.com/jinzhu/gorm"
	nats "github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

// Implementing the `commet.Plugin` interface so it can be run by the `comment.Runtime`

// Constructor to build a plugin that can be used by the runtime
func NewPlugin() commet.DefaultPluginRegistrar {
	return &pluginRegistrar{
		// type flags that enabled differnt features of the pluginRegistrar
		repoType:      commet.PostgresGorm,
		publisherType: commet.Nats,
		// declare the orm
		model: &api.EventORM{},
		// implement business logic
		service: NewService(),
	}
}

// pluginRegistrar is configuration for what features are enabeld in the runtime
type pluginRegistrar struct {
	repoType     commet.RepoType
	consumerType commet.ConsumerType

	model   *api.EventORM
	service *service
}

// Implement the `commet.RepoPluginRegistrar` interface
// so that we can use `Posgres` to store all of the `Event`'s in a
// denormalized format.
func (s *pluginRegistrar) GetRepoType() commet.RepoType {
	return s.repoType
}

func (s *pluginRegistrar) GetModel() interface{} {
	return s.model
}

func (s *pluginRegistrar) RegisterDB(db interface{}) error {
	if db == nil {
		return errors.New("db interface is nil")
	}

	if db, ok := db.(*gorm.DB); ok {
		s.service.rpc.DB = db
	}

	return nil
}

// Implement the `commet.RpcPluginRegistrar` interface because the `EventStore`
// contains a `Create` event for external clients like `web-app`'s, `mobile` app's
// and native desktop applications to create and event known to the whole system
// of services.
func (s *pluginRegistrar) GetIsRpc() bool {
	return true
}

func (s *pluginRegistrar) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	api.RegisterEventServiceServer(server, s.service.rpc)

	return server
}

// Implement the `commet.ProducerPluginRegistrar` interface so that each `Event` can be
// routed to it's correct topic. Currently aggregates are equal to topics.
func (s *pluginRegistrar) GetPublisherType() commet.PublisherType {
	return s.publisherType
}

func (s *pluginRegistrar) RegisterProducer(broker interface{}) error {
	if broker == nil {
		return errors.New("broker connection is nil")
	}

	s.service.pub.broker = broker.(*nats.Conn)

	return nil
}
