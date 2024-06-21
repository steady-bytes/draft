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
	dctl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	project := config.CurrentProject()

	// build out execution path
	rootPath := config.Root()
	apiPath := filepath.Join(rootPath, "api")

	// run docker proto-builder image
	output.Println("Building api...")

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

	// mod update
	output.Println("Running `buf dep update`...")
	config.Cmd = []string{"buf", "dep", "update"}
	err = dctl.RunContainer(ctx, project.API.ContainerName, config, hostConfig, true, true)
	if err != nil {
		return err
	}

	// generate go
	output.Println("Generating Go protos...")
	config.Cmd = []string{"buf", "generate", "--template", "buf.gen.go.yaml"}
	err = dctl.RunContainer(ctx, project.API.ContainerName, config, hostConfig, true, true)
	if err != nil {
		return err
	}

	// generate gotag
	output.Println("Generating Gotag protos...")
	config.Cmd = []string{"buf", "generate", "--template", "buf.gen.gotag.yaml"}
	err = dctl.RunContainer(ctx, project.API.ContainerName, config, hostConfig, true, true)
	if err != nil {
		return err
	}

	// generate web
	output.Println("Generating Web protos...")
	config.Cmd = []string{"npx", "buf", "generate", "--template", "buf.gen.web.yaml"}
	err = dctl.RunContainer(ctx, project.API.ContainerName, config, hostConfig, true, true)
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
	return err
}
