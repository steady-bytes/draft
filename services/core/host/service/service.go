package service

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/rs/zerolog/log"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
	"google.golang.org/grpc"

	ginzerolog "github.com/dn365/gin-zerolog"
	"github.com/gin-gonic/gin"
	"github.com/supertokens/supertokens-golang/recipe/dashboard"
	"github.com/supertokens/supertokens-golang/recipe/emailpassword"
	"github.com/supertokens/supertokens-golang/recipe/session"
	"github.com/supertokens/supertokens-golang/supertokens"
)

// Implementing the `draft.Plugin` interface so it can be run as a plugin to the draft Runtime
type gateway struct {
	*draft.DefaultRuntimeBuilder
}

// Constructor to build a plugin that can be used by the runtime
func New() draft.Default {
	return &gateway{}
}

func (g *gateway) HTTP() *gin.Engine {
	apiBasePath := "/auth"
	websiteBasePath := "/auth"
	if err := supertokens.Init(supertokens.TypeInput{
		Supertokens: &supertokens.ConnectionInfo{
			// https://try.supertokens.com is for demo purposes. Replace this with the address of your core instance (sign up on supertokens.com), or self host a core.
			ConnectionURI: "http://localhost:3567",
			APIKey:        "some_key",
		},
		AppInfo: supertokens.AppInfo{
			AppName:         "draft",
			APIDomain:       "http://localhost:10000",
			WebsiteDomain:   "http://localhost:10000",
			APIBasePath:     &apiBasePath,
			WebsiteBasePath: &websiteBasePath,
		},
		RecipeList: []supertokens.Recipe{
			emailpassword.Init(nil),
			session.Init(nil),
			dashboard.Init(nil),
		},
	}); err != nil {
		log.Panic().Msg("failed to start supertokens")
	}

	// gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// crash safety
	r.Use(gin.Recovery())
	// logging
	r.Use(ginzerolog.Logger("gateway"))

	// cors
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:10000", "http://localhost:10000"},
		AllowMethods: []string{"GET", "POST", "DELETE", "PUT", "OPTIONS"},
		AllowHeaders: append([]string{"content-type"},
			supertokens.GetAllCORSHeaders()...),
		AllowCredentials: true,
	}))

	r.Use(func(c *gin.Context) {
		supertokens.Middleware(http.HandlerFunc(
			func(rw http.ResponseWriter, r *http.Request) {
				c.Next()
			})).ServeHTTP(c.Writer, c.Request)
		c.Abort()
	})

	// a public endpoint just for testing
	r.GET("/auth/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	log.Debug().Msg("open route")

	// a protected endpoint using local middleware
	r.POST("/auth/likecomment", verifySession(nil), likeCommentAPI)
	// host the web-client assets
	r.Static("/assets", "./web-client/build")

	return r
}

func (g *gateway) RPC() *grpc.Server {
	// server := grpc.NewServer()
	// api.RegisterRegistryServer(server, g.service)

	return nil
}

// func (g *gateway) RepoKind() draft.RepoKind {
// 	return draft.NullRepoType
// }

func (g *gateway) BrokerType() draft.BrokerType {
	return draft.NullBrokerType
}
