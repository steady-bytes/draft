package infra

import (
	"github.com/steady-bytes/tools/dctl/docker"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Clean(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	dctl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	err = dctl.RemoveContainerByName(ctx, postgresContainer)
	if err != nil {
		output.Error(err)
	}
	err = dctl.RemoveContainerByName(ctx, mongoContainer)
	if err != nil {
		output.Error(err)
	}
	err = dctl.RemoveContainerByName(ctx, natsContainer)
	if err != nil {
		output.Error(err)
	}
	err = dctl.RemoveContainerByName(ctx, hasuraContainer)
	if err != nil {
		output.Error(err)
	}

	return nil
}
