package api

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/steady-bytes/tools/dctl/config"
	"github.com/steady-bytes/tools/dctl/docker"
	e "github.com/steady-bytes/tools/dctl/execute"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
)

const (
	protoImage = "proto:draft"
)

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctl, err := docker.NewDockerController()
	if err != nil {
		return nil
	}

	// build out execution path
	rootPath := config.Root()
	apiPath := filepath.Join(rootPath, "api")

	err = dctl.BuildImage(ctx, apiPath, protoImage)
	if err != nil {
		return err
	}

	output.Println("Initializing generated code directories...")
	// make go directory
	err = os.MkdirAll(filepath.Join(apiPath, "gen", "go"), os.ModePerm)
	if err != nil {
		return err
	}
	// make web directory
	err = os.MkdirAll(filepath.Join(apiPath, "gen", "web"), os.ModePerm)
	if err != nil {
		return err
	}
	// initialize go mod only if go.mod doesn't already exist
	_, err = os.Stat(filepath.Join(apiPath, "gen", "go", "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		c := exec.Command("go", "mod", "init", config.CurrentProject().Repo + "/api/gen/go")
		c.Dir = filepath.Join(apiPath, "gen", "go")
		err = e.ExecuteCommand(ctx, "go", output.Magenta, c)
		if err != nil {
			return err
		}
	} else if err != nil {
		// if there's an error other than the file not existing, return it
		return err
	}

	// base configuration for docker container runs
	config := &container.Config{
		Image:      protoImage,
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

	// install node modules
	output.Println("Installing node modules...")
	config.Cmd = []string{"npm", "install"}
	err = dctl.RunContainer(ctx, protoContainer, config, hostConfig, true, true)
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
	err = dctl.RunContainer(ctx, protoContainer, config, hostConfig, true, true)
	if err != nil {
		return err
	}

	output.Println("Finished")
	return nil
}
