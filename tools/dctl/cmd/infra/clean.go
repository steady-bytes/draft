package infra

import (
	"github.com/steady-bytes/draft/tools/dctl/docker"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Clean(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	for _, name := range Services {
		err = dockerCtl.RemoveContainerByName(ctx, containerName(name))
		if err != nil {
			output.Error(err)
		}
	}

	return nil
}
