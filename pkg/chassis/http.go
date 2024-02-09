package chassis

import (
	"errors"
	"net/http"

	"github.com/steady-bytes/draft/pkg/logging"
)

type (
	HTTPKind int

	HTTPRegistrar interface {
		RegisterHTTP(interface{}) error
	}
)

const (
	NullHTTPKind HTTPKind = iota
	// Uses the fiber router
	Fiber
	// Used the gin router
	Gin
	// Uses the std library router
	Std
)

var (
	ErrEmptyHttpMultiplexer = errors.New("multiplexer is nil")
	ErrMuxFailedTypecast    = errors.New("incorrect interface")
)

func (c *Runtime) withHTTPHandler(kind HTTPKind, plugin HTTPRegistrar) {
	c.httpKind = kind

	switch c.httpKind {
	case NullHTTPKind:
		return
	case Gin:
		c.withHTTPGin(plugin)
	case Fiber:
		c.withHTTPFiber(plugin)
	case Std:
		c.withHTTPStd(plugin)
	default:
		panic("an invalid http router was provided")
	}

	c.withHTTPGin(plugin)
}

func (c *Runtime) withHTTPFiber(registrar HTTPRegistrar) {
	panic("fiber http router is not fully implemented")
}

func (c *Runtime) withHTTPGin(registrar HTTPRegistrar) {
	// c.gin = registrar.RegisterHTTP()
	c.gin.Use(logging.GinMiddleware([]string{}))
}

func (c *Runtime) withHTTPStd(registrar HTTPRegistrar) {
	// Init and store the http multiplexer
	if c.mux == nil {
		c.mux = http.NewServeMux()
	}

	c.isHTTP = true

	// hand the multiplexer up to the implementing service
	registrar.RegisterHTTP(c.mux)
}
