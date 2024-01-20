package cmd

import (
	"github.com/steady-bytes/tools/dctl/cmd/run"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a single service locally",
	RunE:  run.Run,
}

func init() {
	// add parent
	rootCmd.AddCommand(runCmd)
	// add children
	runCmd.Flags().StringVarP(&run.Domain, "domain", "d", "core", "domain for service")
	runCmd.Flags().StringVarP(&run.Service, "service", "s", "registry", "service to run")
}
