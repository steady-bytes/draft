package cmd

import (
	"github.com/steady-bytes/tools/dctl/cmd/initialize"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new draft project",
	RunE:  initialize.Init,
}

func init() {
	// add parent
	rootCmd.AddCommand(initCmd)
	// add children
	initCmd.Flags().StringVarP(&initialize.Path, "path", "p", ".", "the path to initialize the project in")
	// runCmd.Flags().StringVarP(&run.Service, "service", "s", "registry", "service to run")
}
