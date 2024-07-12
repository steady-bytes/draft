package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/context"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Commands to manage draft contexts",
	Long: `Commands to manage draft contexts. A draft context defines a draft system for dctl to manage.

By default, dctl will choose a context similarly to how git does where the current working
directory informs context selection. If there is a draft workspace config (draft.yaml) within a parent directory that context will be used.
Otherwise dctl will use the default from the dctl config file (you can change this by calling 'dctl context set'). You can always
override the selected context by providing the --context flag on any dctl command.

Any repo with a draft workspace config file (draft.yaml) can be imported into dctl using the
'draft context import' command. You can also initializes a new draft project (context) using the
'draft context init' command.`,
}

var contextInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new draft context (project)",
	Long: `Initialize a new draft context (project).

Run this from the root of a new git repository or provide the path to the root of the repository
as a flag. This command will scaffold out the required directories and configuration files for
the new draft project and import it as a context for dctl to manage.`,
	Example: "dctl context init --path /path/to/new/git/repo",
	RunE:    context.Init,
}

var contextSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set the default context",
	RunE:  context.SetDefault,
}

var contextImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an existing context",
	Long: `Import an existing repository as a draft context for dctl to manage. The
repository must have a valid draft workspace config file at its root (draft.yaml).`,
	Example: "dctl context import --path /home/userA/repos/repoX",
	RunE:    context.Import,
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
