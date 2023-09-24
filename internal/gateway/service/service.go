package service

import (
	"net/http"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	ginzerolog "github.com/dn365/gin-zerolog"
	"github.com/gin-gonic/gin"
)

// Implementing the `draft.Plugin` interface so it can be run by the runtime

// Constructor to build a plugin that can be used by the runtime
func NewService() draft.DefaultPluginRegistrar {
	return &gateway{}
}

type gateway struct {
	*draft.DefaultRuntimeBuilder
}

func (g *gateway) RegisterHTTP() *gin.Engine {
	return NewRouter()
}

func NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginzerolog.Logger(name))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	return r
}

func (g *gateway) GetRepoType() draft.RepoType {
	return draft.NullRepoType
}

func (g *gateway) GetBrokerType() draft.BrokerType {
	return draft.NullBrokerType
}
