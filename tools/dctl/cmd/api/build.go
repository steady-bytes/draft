package api

import (
	"fmt"
	"os/exec"
	"os/user"
	"path/filepath"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/docker"
	e "github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/spf13/cobra"
)

func Build(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	dctx := config.GetContext()

	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	var image string
	if ImageName != "" {
		image = ImageName
		output.Print("Using image: %s", image)
	} else {
		image = dctx.API.ImageName
	}

	// build out execution path
	rootPath := dctx.Root
	apiPath := filepath.Join(rootPath, "api")

	// run docker proto-builder image
	output.Print("Building api...")

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

	// mod update
	output.Print("Running `buf dep update`...")
	config.Cmd = []string{"buf", "dep", "update"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	// generate go
	output.Print("Generating Go protos...")
	config.Cmd = []string{"buf", "generate", "--template", "buf.gen.go.yaml"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	// generate gotag
	output.Print("Generating Gotag protos...")
	config.Cmd = []string{"buf", "generate", "--template", "buf.gen.gotag.yaml"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	// generate web
	output.Print("Generating Web protos...")
	config.Cmd = []string{"npx", "buf", "generate", "--template", "buf.gen.web.yaml"}
	err = dockerCtl.RunContainer(ctx, dockerCtl.GenerateContainerName(), config, hostConfig, true)
	if err != nil {
		return err
	}

	// extra steps for go
	c := exec.Command("go", "mod", "tidy")
	c.Dir = apiPath
	err = e.ExecuteCommand(ctx, "go", output.Green, c)
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
	return err
}
