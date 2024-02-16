package main

import (
	"fmt"
	"net/http"

	"github.com/steady-bytes/draft/pkg/brokers/amqp"
	"github.com/steady-bytes/draft/pkg/brokers/nats"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/logrus"
	"github.com/steady-bytes/draft/pkg/repositories/badger"
	"github.com/steady-bytes/draft/pkg/repositories/clickhouse"
	"github.com/steady-bytes/draft/pkg/repositories/mongo"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/gorm"
	"github.com/steady-bytes/draft/pkg/secrets/vault"

	"github.com/gin-gonic/gin"
)

type Data struct {
	String string
}

var logger chassis.Logger

func main() {
	// logger = zerolog.New()
	logger = logrus.New()

	defer chassis.New(logger).
		WithBroker(amqp.New("")).
		WithBroker(nats.New("")).
		WithRepository(badger.New()).
		WithRepository(clickhouse.New("")).
		WithRepository(mongo.New("")).
		WithRepository(bun.New("")).
		WithRepository(gorm.New("")).
		WithSecretStore(vault.New("")).
		Start()
}

func method1(c *gin.Context) {
	l := logger.WithField("key1", "value1")
	l.Info("hello 1")
	err := method2(l)
	l.WrappedError(err, "failed while processing request")
	c.String(http.StatusInternalServerError, chassis.Unwrap(err).Error())
}

func method2(l chassis.Logger) error {
	l = l.WithField("key2", "value2")
	l.Info("hello 2")
	return l.Wrap(method3(l))
}

func method3(l chassis.Logger) error {
	l = l.WithField("key3", "value3")
	l.Info("hello 3")
	return l.Wrap(fmt.Errorf("failed to do something"))
}
