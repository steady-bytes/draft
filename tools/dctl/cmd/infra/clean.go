package infra

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steady-bytes/draft/tools/dctl/docker"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Clean(cmd *cobra.Command, args []string) (err error) {
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

		err = dockerCtl.RemoveContainerByName(ctx, containerName(name))
		if err != nil {
			output.Error(err)
		}

		if config.configFile != nil {
			output.Print("Deleting configuration file for: %s", name)
			home, err := os.UserHomeDir()
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}

			dirName := filepath.Join(home, ".config", "dctl", "infra")
			fileName := filepath.Join(dirName, fmt.Sprintf("%s.yaml", name))
			err = os.Remove(fileName)
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}
		}

		if config.mountPoint != nil {
			output.Print("Deleting volume for: %s", name)
			home, err := os.UserHomeDir()
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}

			dirName := filepath.Join(home, ".config", "dctl", "infra", name)
			err = os.RemoveAll(dirName)
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}
		}
	}

	return nil
}
