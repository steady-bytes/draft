package main

import (
	"github.com/steady-bytes/draft/pkg/brokers/amqp"
	"github.com/steady-bytes/draft/pkg/brokers/nats"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/repositories/badger"
	"github.com/steady-bytes/draft/pkg/repositories/clickhouse"
	"github.com/steady-bytes/draft/pkg/repositories/mongo"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/gorm"
	"github.com/steady-bytes/draft/pkg/secrets/vault"
)

func main() {
	defer chassis.New().
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
