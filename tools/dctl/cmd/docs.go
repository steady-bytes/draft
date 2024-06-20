package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var (
	docsDirectory string
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate docs for dctl",
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Println("Writing docs to: %s", docsDirectory)
		err := doc.GenMarkdownTree(rootCmd, docsDirectory)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	// add parent
	rootCmd.AddCommand(docsCmd)
	docsCmd.Flags().StringVarP(&docsDirectory, "out", "o", "./docs", "the directory to write docs to")
}
