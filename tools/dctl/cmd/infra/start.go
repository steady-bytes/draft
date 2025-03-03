package infra

import (
	"context"
	"time"

	"github.com/steady-bytes/draft/tools/dctl/docker"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

var (
	Follow   bool
	Services []string
)

func Start(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	for _, name := range Services {
		config, err := getInfraConfig(name)
		if err != nil {
			output.Error(err)
			return err
		}

		// new context so it doesn't get canceled by the user on ctrl+c if Follow is true
		c := context.Background()
		// delay hasura start until databases are ready (wait 5 seconds)
		if name == "hasura" {
			time.Sleep(5 * time.Second)
		}

		id, err := dockerCtl.StartContainer(c, containerName(name), config.containerConfig, config.hostConfig, Follow)
		if err != nil {
			return err
		}
		if Follow {
			defer stop(c, dockerCtl, id)
		}
	}

	if Follow {
		<-ctx.Done()
	} else {
		for _, name := range Services {
			err = checkStatus(ctx, dockerCtl, name, containerName(name))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// stop is a helper function that stops a container and logs any error so as to be easy to use
// in a defer statement or goroutine.
func stop(ctx context.Context, dctl docker.DockerController, id string) {
	err := dctl.StopContainer(ctx, id)
	if err != nil {
		output.Error(err)
	}
}
