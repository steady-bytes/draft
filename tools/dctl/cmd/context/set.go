package context

import (
	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

var (
	Context string
)

func SetDefault(cmd *cobra.Command, args []string) error {
	err := config.SetDefaultContext(Context)
	if err != nil {
		return err
	}
	output.Println("The default context is now: %s", Context)
	return nil
}
