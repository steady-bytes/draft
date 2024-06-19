package cmd

import (
	"github.com/steady-bytes/tools/dctl/cmd/release"

	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Commands for releasing Draft components",
}

var releaseModule = &cobra.Command{
	Use:     "module",
	Aliases: []string{"mod"},
	Short:   "Release a Go module",
	Long: `Release a Go module using a git tag. This will check the latest tag for the given module
and will ask how you would like to increment the semantic version (major.minor.patch). It will create
a git tag with the new version and push it to the git origin.`,
	RunE: release.Module,
}

func init() {
	// add parent
	rootCmd.AddCommand(releaseCmd)
	// add children
	releaseCmd.AddCommand(releaseModule)
	releaseModule.Flags().StringVarP(&release.Path, "path", "p", "", "path of Go module to release (e.g. pkg/chassis or tools/dctl)")
}
