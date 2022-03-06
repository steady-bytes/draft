package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(eventStore)
	// eventStore.Flags().Int32VarP(&port, "port", "p", 50002, "port override, by default the port is 50002")
}

var querier = &cobra.Command{
	Use:   "querier",
	Short: "run the querier component of firegraph.",
	RunE: func(cmd *cobra.Command, args []string) error {

		/* err := Runtime.WithDefaultBuilder(qu.NewPlugin()).Start()
		 * if err != nil {
		 *   panic(err)
		 * } */

		return nil
	},
}
