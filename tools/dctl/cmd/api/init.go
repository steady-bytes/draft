package api

import (
	"fmt"
	"os/user"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/docker"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	project := config.CurrentProject()

	// build out execution path
	rootPath := config.Root()
	apiPath := filepath.Join(rootPath, "api")

	err = dctl.PullImage(ctx, project.API.ImageName)
	if err != nil {
		return err
	}

	// base configuration for docker container runs
	config := &container.Config{
		Image:      project.API.ImageName,
		WorkingDir: "/workspace",
	}
	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: apiPath,
				Target: "/workspace",
			},
		},
	}

	// initialize go module
	output.Println("Initializing go module...")
	config.Cmd = []string{"go", "mod", "init", fmt.Sprintf("%s/api", project.Repo)}
	err = dctl.RunContainer(ctx, dctl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	// install node modules
	output.Println("Installing node modules...")
	config.Cmd = []string{"npm", "install", "--no-fund"}
	err = dctl.RunContainer(ctx, project.API.ContainerName, config, hostConfig, true, true)
	if err != nil {
		return err
	}

	// make sure to chown the files to the current user
	output.Println("Correcting file permissions...")
	u, err := user.Current()
	if err != nil {
		return err
	}
	config.Cmd = []string{"chown", "-R", fmt.Sprintf("%s:%s", u.Uid, u.Gid), "/workspace"}
	err = dctl.RunContainer(ctx, project.API.ContainerName, config, hostConfig, true, true)
	if err != nil {
		return err
	}

	output.Println("Finished")
	return nil
}
