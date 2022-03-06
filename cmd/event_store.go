package cmd

import (
	commet "commet"

	es "event_store"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(eventStore)
	eventStore.Flags().Int32VarP(&port, "port", "p", 50001, "port override, by default the port is 50001")
}

var eventStore = &cobra.Command{
	Use:   "event_store",
	Short: "run the event store component of `firegraph`.",
	RunE: func(cmd *cobra.Command, args []string) error {

		options := commet.DefaultBuilderToggles{
			// isRpc:       true,
			// isPublisher: true,
		}

		Runtime.WithDefaultBuilder(es.NewPlugin(), options).Start()

		return nil
	},
}
