package context

import (
	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

var (
	Project string
)

func Set(cmd *cobra.Command, args []string) error {
	err := config.SetContext(Project)
	if err != nil {
		return err
	}
	output.Println("The current context is now: %s", Project)
	return nil
}
