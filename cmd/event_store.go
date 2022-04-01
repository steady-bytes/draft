package cmd

import (
	es "github.com/steady-bytes/draft/internal/event_store"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(eventStore)
	eventStore.Flags().Int32VarP(&rpcPort, "rpc_port", "r", 50001, "rpc port override, by default the rpc port is 50001")
}

var eventStore = &cobra.Command{
	Use:   "event_store",
	Short: "run the event store component of `draft`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name = "event_store"

		if err := Runtime.DefaultBuilder(es.NewPlugin()).Start(); err != nil {
			panic(err)
		}

		return nil
	},
}
