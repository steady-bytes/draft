package infra

import (
	"fmt"

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

	for _, name := range Services {
		config, ok := infraConfigs[name]
		if !ok {
			output.Error(fmt.Errorf("invalid infra service name: %s", name))
		}
		output.Println("Pulling Docker image for: %s", name)
		err = dctl.PullImage(ctx, config.containerConfig.Image)
		if err != nil {
			return err
		}
	}

	output.Println("Finished")
	return nil
}
