package chassis

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/steady-bytes/draft/pkg/logging"
)

type Runtime struct {
	config Config
	logger logging.Logger

	brokers      []Broker
	repositories []Repository
	secretStores []SecretStore

	// network toggles
	isRPC                     bool
	rpcReflectionServiceNames []string
	isHTTP                    bool
	// multiplexer
	mux *http.ServeMux
	// router options
	gin      *gin.Engine
	httpKind HTTPKind

	consensusKind        ConsensusKind
	raftAdvertiseAddress *net.TCPAddr
}

func New() *Runtime {
	rt := &Runtime{
		config: LoadConfig(),
		isRPC:  false,
		isHTTP: false,
	}
	var formatter *logging.Formatter
	if rt.config.Env() != "local" {
		formatter = &logging.Formatter{
			Line:    true,
			Package: true,
			File:    true,
			ChildFormatter: &logrus.JSONFormatter{
				DisableHTMLEscape: true,
			},
		}
	}
	rt.logger = logging.CreateLogger(rt.config.GetString("service.logging.level"), rt.config.Name(), formatter)
	return rt
}
