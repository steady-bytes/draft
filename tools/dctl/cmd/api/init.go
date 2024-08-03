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

var (
	BuildImage bool
	ImageName  string
)

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctx := config.GetContext()

	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	// build out execution path
	rootPath := dctx.Root
	apiPath := filepath.Join(rootPath, "api")

	var image string
	if BuildImage {
		image = ImageName
		err = dockerCtl.BuildImage(ctx, apiPath, image)
	} else {
		image = dctx.API.ImageName
		err = dockerCtl.PullImage(ctx, image)
	}
	if err != nil {
		return err
	}

	// base configuration for docker container runs
	config := &container.Config{
		Image:      image,
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

	// install node modules
	output.Print("Installing node modules...")
	config.Cmd = []string{"npm", "install", "--no-fund"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	// make sure to chown the files to the current user
	output.Print("Correcting file permissions...")
	u, err := user.Current()
	if err != nil {
		return err
	}
	config.Cmd = []string{"chown", "-R", fmt.Sprintf("%s:%s", u.Uid, u.Gid), "/workspace"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	output.Print("Finished")
	return nil
}
