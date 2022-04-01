package cmd

import (
	rg "github.com/steady-bytes/draft/internal/registry"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(registry)
	registry.Flags().Int32VarP(&rpcPort, "rpc_port", "r", 50002, "rpc port override, by default the rpc port is 50002")
	registry.Flags().Int32VarP(&httpPort, "http_port", "p", 40002, "http port override, by default the http port is 40002")
}

var registry = &cobra.Command{
	Use:   "registry",
	Short: "run the registry component of `draft`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name = "registry"

		if err := Runtime.AggregateBuilder(rg.NewPlugin()).Start(); err != nil {
			panic(err)
		}

		return nil
	},
}
