package cmd

import (
	wr "github.com/steady-bytes/draft/internal/writer"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(writer)
	writer.Flags().Int32VarP(&rpcPort, "rpc_port", "r", 50000, "rpc port override, by default the rpc port is 50000")
}

var writer = &cobra.Command{
	Use:   "writer",
	Short: "run the writer interface of `draft`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name = "writer"

		if err := Runtime.DefaultBuilder(wr.NewPlugin()).Start(); err != nil {
			panic(err)
		}

		return nil
	},
}
