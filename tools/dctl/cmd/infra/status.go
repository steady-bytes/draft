package infra

import (
	"context"

	"github.com/steady-bytes/draft/tools/dctl/docker"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/docker/docker/api/types"
	"github.com/spf13/cobra"
)

func Status(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return err
	}

	for _, name := range Services {
		err = checkStatus(ctx, dockerCtl, name, containerName(name))
		if err != nil {
			return err
		}
	}

	return nil
}

func checkStatus(ctx context.Context, dockerCtl docker.DockerController, friendlyName, containerName string) error {
	container, err := dockerCtl.GetContainerByName(ctx, containerName)
	if err != nil {
		return err
	}
	if container == nil {
		container = &types.Container{
			Status: "No container found",
			State:  "no state",
		}
	}
	output.PrintlnWithNameAndColor(friendlyName, "status: %s (%s)", getColor(container.State), container.Status, container.State)
	return nil
}

func getColor(state string) output.Color {
	switch state {
	case "running":
		return output.Green
	case "exited":
		return output.Yellow
	default:
		return output.Red
	}
}
