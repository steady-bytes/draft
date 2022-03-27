package cmd

import (
	wr "github.com/steady-bytes/draft/internal/writer"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(writer)
	writer.Flags().Int32VarP(&port, "port", "p", 50000, "port override, by default the port is 50000")
}

var writer = &cobra.Command{
	Use:   "writer",
	Short: "run the writer interface of `draft`.",
	RunE: func(cmd *cobra.Command, args []string) error {

		if err := Runtime.RpcBuilder(wr.NewPlugin()).Start(); err != nil {
			panic(err)
		}

		return nil
	},
}
