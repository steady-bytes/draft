package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/infra"

	"github.com/spf13/cobra"
)

var infraCmd = &cobra.Command{
	Use:     "infra",
	Aliases: []string{"infrastructure"},
	Short:   "Manage all local draft infra services (Docker containers)",
	Long: `Manage all local draft infra services (Docker containers). dctl runs all local
infrastructure as Docker containers and you can manage their lifecycle with the commands
below this one.

Note that you can specify which services to operate on with any of the other commands
using the flag --services:

dctl infra start --services 'postgres,nats,hasura'`,
}

var infraCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up infra resources (Docker containers)",
	RunE:  infra.Clean,
}

var infraInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Pull Docker images required for draft infra",
	RunE:  infra.Init,
}

var infraStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Run draft infra Docker containers",
	RunE:  infra.Start,
}

var infraStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop running draft infra Docker containers",
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
	infraCleanCmd.Flags().StringSliceVarP(&infra.Services, "services", "s", []string{"nats", "postgres"}, "infra services to act on")
	infraCmd.AddCommand(infraInitCmd)
	infraInitCmd.Flags().StringSliceVarP(&infra.Services, "services", "s", []string{"nats", "postgres"}, "infra services to act on")
	infraCmd.AddCommand(infraStartCmd)
	infraStartCmd.Flags().BoolVarP(&infra.Follow, "follow", "f", false, "whether or not to follow the output of the infra docker containers (true/FALSE)")
	infraStartCmd.Flags().StringSliceVarP(&infra.Services, "services", "s", []string{"nats", "postgres"}, "infra services to act on")
	infraCmd.AddCommand(infraStopCmd)
	infraStopCmd.Flags().StringSliceVarP(&infra.Services, "services", "s", []string{"nats", "postgres"}, "infra services to act on")
	infraCmd.AddCommand(infraStatusCmd)
	infraStatusCmd.Flags().StringSliceVarP(&infra.Services, "services", "s", []string{"nats", "postgres"}, "infra services to act on")
}
