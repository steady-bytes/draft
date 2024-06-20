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
	Use:   "build",
	Short: "Compile all protobuf files",
	RunE:  api.Build,
}

var apiCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up all compiled (generated) protobuf files",
	RunE:  api.Clean,
}

var apiInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Build the Docker image that compiles protobufs",
	RunE:  api.Init,
}

func init() {
	// add parent
	rootCmd.AddCommand(apiCmd)
	// add children
	apiCmd.AddCommand(apiBuildCmd)
	apiCmd.AddCommand(apiCleanCmd)
	apiCmd.AddCommand(apiInitCmd)
}
