package api

import (
	"errors"
	"fmt"
	"os"
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
	dctx := config.CurrentContext()

	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	// build out execution path
	rootPath := dctx.Root
	apiPath := filepath.Join(rootPath, "api")

	err = dockerCtl.PullImage(ctx, dctx.API.ImageName)
	if err != nil {
		return err
	}

	// base configuration for docker container runs
	config := &container.Config{
		Image:      dctx.API.ImageName,
		WorkingDir: "/workspace",
	}
	hostConfig := &container.HostConfig{
		AutoRemove: true,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: apiPath,
				Target: "/workspace",
			},
		},
	}

	// initialize go module (if it doesn't already exist)
	goModPath := filepath.Join(dctx.Repo, "api")
	if _, err := os.Stat(goModPath); errors.Is(err, os.ErrNotExist) {
		output.Println("Initializing go module...")
		config.Cmd = []string{"go", "mod", "init", }
		err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
		if err != nil {
			return err
		}
	}

	// install node modules
	output.Println("Installing node modules...")
	config.Cmd = []string{"npm", "install", "--no-fund"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
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
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	output.Println("Finished")
	return nil
}
