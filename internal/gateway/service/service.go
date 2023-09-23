package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
)

// Implementing the `draft.Plugin` interface so it can be run by the runtime

// Constructor to build a plugin that can be used by the runtime
func NewService() draft.DefaultPluginRegistrar {
	g := gateway{
		// model:      NewModel(),
		// controller: NewController(),
		handler: NewHandler(),
		// consumer:   NewConsumer(),
		// publisher:  NewPublisher(),
	}

	return g
}

type gateway struct {
	*draft.DefaultRuntimeBuilder
	// controller *controller
	handler *handler
}

type handler struct {
	// route *gin.Router
}

func NewHandler() *handler {
	return &handler{}
}

func Run() {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "ping",
		})
	})

	r.Run()
}

// Override the `ReisterDB` default behavior for the `DefaultRuntimebuilder`
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

// Override the `RegisterRPC` default behavior for the `DefaultRuntimeBuilder`
// func (s *eventStorePlugin) RegisterRPC() *grpc.Server {
// 	server := grpc.NewServer()
// 	api.RegisterEventStoreServer(server, s.service.rpc)

// 	return server
// }

// Override the `RegisterBroker` default behavior for the `DefaultRuntimeBuilder`
// func (s *eventStorePlugin) RegisterBroker(broker interface{}) error {
// 	if broker == nil {
// 		return errors.New("broker connection is nil")
// 	}

// 	s.service.msg.broker = broker.(*nats.Conn)

// 	return nil
// }
