package cmd

import (
	"github.com/steady-bytes/tools/dctl/cmd/project"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Commands to manage draft projects",
}

var projectInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new draft project",
	RunE:  project.Init,
}

var projectSetCmd = &cobra.Command{
	Use: "set",
	Short: "Set the current project",
	RunE: project.Set,
}

var projectImportCmd = &cobra.Command{
	Use: "import",
	Short: "Import an existing project",
	RunE: project.Import,
}

func init() {
	// add parent
	rootCmd.AddCommand(projectCmd)
	// add children
	projectCmd.AddCommand(projectInitCmd)
	projectInitCmd.Flags().StringVarP(&project.Path, "path", "p", ".", "the path to initialize the project in")
	projectCmd.AddCommand(projectSetCmd)
	projectSetCmd.Flags().StringVarP(&project.Project, "project", "p", ".", "the project to make currently active")
	projectCmd.AddCommand(projectImportCmd)
	projectImportCmd.Flags().StringVarP(&project.Path, "path", "p", ".", "the path to initialize the project in")
}
