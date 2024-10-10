package main

import (
	consumer "github.com/steady-bytes/draft/services/core/catalyst/consumer"
	producer "github.com/steady-bytes/draft/services/core/catalyst/producer"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
)

func main() {
	var (
		logger = zerolog.New()

		consumerController = consumer.NewController()
		consumerRPC        = consumer.NewRPC(logger, consumerController)

		producerController = producer.NewController()
		producerRPC        = producer.NewRPC(logger, producerController)
	)

	defer chassis.New(logger).
		WithRPCHandler(consumerRPC).
		WithRPCHandler(producerRPC).
		Start()
}
