package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/config"

	"github.com/spf13/cobra"
)

var configCommand = &cobra.Command{
	Use:   "config",
	Short: "Initialize the dctl configuration",
	RunE: config.Config,
}

func init() {
	// add parent
	rootCmd.AddCommand(configCommand)
}
