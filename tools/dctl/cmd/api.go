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
	Short: "Generate code from all protobuf files",
	Long: `Generate code from all protobuf files. Make sure to run 'dctl api init' before running this as
this command requires the setup performed by init.

Note that you can override the image defined by the workspace config with the --image flag:

dctl api build --image draft-proto-builder:local`,
	Example: "dctl api build",
	PreRunE: requireWorkspace,
	RunE:    api.Build,
}

var apiInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a draft workspace for generating code from protos",
	Long: `Initialize a draft workspace for generating code from protos. This involves
pulling or building the protobuf builder Docker image and installing Node modules.

By default dctl will try and pull the Docker image defined in the draft workspace config but you can tell it
to build the image locally instead:

dctl api init --build true --image draft-proto-builder:local

Be sure to include the --image flag on any subsequent calls to build:

dctl api build --image draft-proto-builder:local`,
	Example: "dctl api init",
	PreRunE: requireWorkspace,
	RunE:    api.Init,
}

func init() {
	// add parent
	rootCmd.AddCommand(apiCmd)
	// add children
	apiCmd.AddCommand(apiBuildCmd)
	apiBuildCmd.Flags().StringVarP(&api.ImageName, "image", "i", "", "override the builder image name from the workspace config")
	apiCmd.AddCommand(apiInitCmd)
	apiInitCmd.Flags().BoolVarP(&api.BuildImage, "build", "b", false, "build the Docker image instead of pulling it")
	apiInitCmd.Flags().StringVarP(&api.ImageName, "image", "i", "", "required when --build=true. defines the name of the image to build")
}
