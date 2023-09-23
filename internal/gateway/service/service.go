package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"
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
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "ping",
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
