package cmd

import (
	"github.com/spf13/cobra"

	"github.com/steady-bytes/draft/tools/dctl/cmd/broker"
)

var brokerCmd = &cobra.Command{
	Use:     "broker",
	Aliases: []string{"b"},
	Short:   "Commands for interacting with catalyst the message broker",
}

var brokerConsumeCmd = &cobra.Command{
	Example: "dctl broker consume",
	Use:     "consume",
	Aliases: []string{"c"},
	Short:   "Consume messages from a specific domain. If * is used all message will be consumed",
	RunE:    broker.Consume,
}

var brokerProduceCmd = &cobra.Command{
	Example: "dctl broker produce",
	Use:     "produce",
	Aliases: []string{"p"},
	Short:   "produce an event in catalyst",
	RunE:    broker.Produce,
}

var (
	p         = ""
	name      = "domain"
	shorthand = "d"
)

func init() {
	rootCmd.AddCommand(brokerCmd)

	brokerCmd.AddCommand(brokerConsumeCmd)
	brokerCmd.AddCommand(brokerProduceCmd)
}
