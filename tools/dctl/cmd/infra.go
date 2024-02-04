package cmd

import (
	"github.com/steady-bytes/tools/dctl/cmd/infra"

	"github.com/spf13/cobra"
)

var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "Manage all local draft infra (Docker containers)",
}

var infraCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up infra resources (Docker containers)",
	RunE:  infra.Clean,
}

var infraInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Build all custom draft infra Docker images",
	RunE:  infra.Init,
}

var infraStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start all draft infra locally",
	RunE:  infra.Start,
}

var infraStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running draft infra",
	RunE:  infra.Stop,
}

var infraStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of all local infra",
	RunE:  infra.Status,
}

func init() {
	// add parent
	rootCmd.AddCommand(infraCmd)
	// add children
	infraCmd.AddCommand(infraCleanCmd)
	infraCmd.AddCommand(infraInitCmd)
	infraCmd.AddCommand(infraStartCmd)
	infraStartCmd.Flags().BoolVarP(&infra.Follow, "follow", "f", false, "whether or not to follow the output of the infra docker containers (true/false)")
	infraCmd.AddCommand(infraStopCmd)
	infraCmd.AddCommand(infraStatusCmd)
}
