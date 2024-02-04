package infra

import (
	"github.com/steady-bytes/tools/dctl/docker"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	for name, config := range infraConfigs {
		output.Println("Pulling Docker image for: %s", name)
		err = dctl.PullImage(ctx, config.containerConfig.Image)
		if err != nil {
			output.Error(err)
			return err
		}
	}

	output.Println("Finished")
	return nil
}
