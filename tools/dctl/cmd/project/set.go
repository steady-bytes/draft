package project

import (
	"github.com/steady-bytes/tools/dctl/config"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
)

var (
	Project string
)

func Set(cmd *cobra.Command, args []string) error {
	err := config.SetProject(Project)
	if err != nil {
		return err
	}
	output.Println("The current project is now: %s", Project)
	return nil
}
