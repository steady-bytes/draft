package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/context"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Commands to manage draft contexts",
}

var contextInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new draft context",
	RunE:  context.Init,
}

var contextSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set the default context",
	RunE:  context.SetDefault,
}

var contextImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an existing context",
	RunE:  context.Import,
}

func init() {
	// add parent
	rootCmd.AddCommand(contextCmd)
	// add children
	contextCmd.AddCommand(contextInitCmd)
	contextInitCmd.Flags().StringVarP(&context.Path, "path", "p", ".", "the path to initialize the context in")
	contextCmd.AddCommand(contextSetCmd)
	contextSetCmd.Flags().StringVarP(&context.Context, "default", "d", "", "the context to make the default")
	contextCmd.AddCommand(contextImportCmd)
	contextImportCmd.Flags().StringVarP(&context.Path, "path", "p", ".", "the path to import the context from")
}
