package cmd

import (
	rg "github.com/steady-bytes/draft/internal/registrar"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(registrar)
	registrar.Flags().Int32VarP(&rpcPort, "rpc_port", "r", 50000, "rpc port override, by default the rpc port is 50002")
	registrar.Flags().Int32VarP(&httpPort, "http_port", "p", 40000, "http port override, by default the http port is 40002")
}

var registrar = &cobra.Command{
	Use:   "registrar",
	Short: "run the registrar component of `draft`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name = "registrar"

		if err := Runtime.DefaultBuilder(rg.NewPlugin()).Start(); err != nil {
			panic(err)
		}

		return nil
	},
}
