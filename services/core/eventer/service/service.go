package service

// import (
// 	"errors"

// 	api "github.com/steady-bytes/draft/api/go"
// 	draft "github.com/steady-bytes/draft/pkg/draft-runtime"

// 	"github.com/jinzhu/gorm"
// 	nats "github.com/nats-io/nats.go"
// 	"google.golang.org/grpc"
// )

// // Implementing the `draft.Plugin` interface so it can be run by the runtime

// // Constructor to build a plugin that can be used by the runtime
// func NewService() draft.DefaultPluginRegistrar {
// 	return &service{
// 		// model:      NewModel(),
// 		// controller: NewController(),
// 		// handler: NewHandler(),
// 		// consumer:   NewConsumer(),
// 		// publisher:  NewPublisher(),
// 	}
// }

// // eventStorePlugin is configuration for what features are enabeld in the runtime
// type eventStore struct {
// 	*draft.DefaultRuntimeBuilder
// 	// controller *controller
// 	handler *handler
// }

// // Override the `ReisterDB` default behavior for the `DefaultRuntimebuilder`
// func (s *eventStorePlugin) RegisterDB(db interface{}) error {
// 	if db == nil {
// 		return errors.New("db interface is nil")
// 	}

// 	if db, ok := db.(*gorm.DB); ok {
// 		db = db.AutoMigrate(&api.EventORM{})
// 		s.service.rpc.DB = db
// 	}

// 	return nil
// }

// // Override the `RegisterRPC` default behavior for the `DefaultRuntimeBuilder`
// func (s *eventStorePlugin) RegisterRPC() *grpc.Server {
// 	server := grpc.NewServer()
// 	api.RegisterEventStoreServer(server, s.service.rpc)

// 	return server
// }

// // Override the `RegisterBroker` default behavior for the `DefaultRuntimeBuilder`
// func (s *eventStorePlugin) RegisterBroker(broker interface{}) error {
// 	if broker == nil {
// 		return errors.New("broker connection is nil")
// 	}

// 	s.service.msg.broker = broker.(*nats.Conn)

// 	return nil
// }

// func (s *eventStor)

// type service struct {
// 	rpc *api.EventStoreDefaultServer
// 	msg *eventStoreMessagePublisher
// }

// func NewService() *service {
// 	return &service{
// 		rpc: NewRPC(),
// 		msg: NewMessagePublisher(),
// 	}
// }
