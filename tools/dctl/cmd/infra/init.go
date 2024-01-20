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

	// pull needed images
	output.Println("Pulling NATS Docker image...")
	err = dctl.PullImage(ctx, natsImage)
	if err != nil {
		output.Error(err)
		return err
	}
	output.Println("Pulling Mongo Docker image...")
	err = dctl.PullImage(ctx, mongoImage)
	if err != nil {
		output.Error(err)
		return err
	}
	output.Println("Pulling Postgres Docker image...")
	err = dctl.PullImage(ctx, postgresImage)
	if err != nil {
		output.Error(err)
		return err
	}
	output.Println("Pulling Hasura Docker image...")
	err = dctl.PullImage(ctx, hasuraImage)
	if err != nil {
		output.Error(err)
		return err
	}

	output.Println("Finished")
	return nil
}
