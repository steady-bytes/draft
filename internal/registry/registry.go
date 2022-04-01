package registry

import (
	"errors"
	"fmt"

	api "github.com/steady-bytes/draft/api/gen/go"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime"

	fiber "github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/handlebars"
	"github.com/jinzhu/gorm"
	"google.golang.org/grpc"
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

func (r *registry) IsRpc() bool {
	return true
}

func (r *registry) RegisterRPC() *grpc.Server {
	server := grpc.NewServer()
	api.RegisterRegistryServer(server, r.service)

	return server
}

func (r *registry) IsHttp() bool {
	return true
}

func (r *registry) RegisterHTTP() *fiber.App {
	engine := handlebars.New("./views", ".hbs")

	httpMux := fiber.New(fiber.Config{
		Views: engine,
	})

	httpMux.Get("/hello", hello)

	httpMux.Get("/", func(c *fiber.Ctx) error {
		// Render index within layouts/main
		return c.Render("index", fiber.Map{
			"Title": "Hello, World from the layout!",
		}, "layouts/main")
	})

	return httpMux
}

func hello(c *fiber.Ctx) error {
	return c.SendString("Hello, World!")
}
