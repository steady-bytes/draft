package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/api"

	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Commands for managing the draft API (protobufs)",
}

var apiBuildCmd = &cobra.Command{
	Use:     "build",
	Short:   "Compile all protobuf files",
	PreRunE: requireWorkspace,
	RunE:    api.Build,
}

var apiInitCmd = &cobra.Command{
	Use:     "init",
	Short:   "Build the Docker image that compiles protobufs",
	PreRunE: requireWorkspace,
	RunE:    api.Init,
}

func init() {
	// add parent
	rootCmd.AddCommand(apiCmd)
	// add children
	apiCmd.AddCommand(apiBuildCmd)
	apiCmd.AddCommand(apiInitCmd)
}
