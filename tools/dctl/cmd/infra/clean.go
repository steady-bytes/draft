package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
			continue
		}

		err = dockerCtl.StopContainerByName(ctx, containerName(name))
		if err != nil && !strings.Contains(err.Error(), "not found") {
			output.Error(err)
		}

		err = dockerCtl.RemoveContainerByName(ctx, containerName(name))
		if err != nil && !strings.Contains(err.Error(), "not found") {
			output.Error(err)
		}

		if Full {
			if config.configFile != nil {
				output.Print("Deleting configuration file for: %s", name)
				home, err := os.UserHomeDir()
				if err != nil {
					output.Error(err)
					continue
				}

				dirName := filepath.Join(home, ".config", "dctl", "infra")
				fileName := filepath.Join(dirName, fmt.Sprintf("%s.yaml", name))
				err = os.Remove(fileName)
				if err != nil && !os.IsNotExist(err) {
					output.Error(err)
					continue
				}
			}

			if config.mountPoint != nil {
				output.Print("Deleting volume for: %s", name)
				home, err := os.UserHomeDir()
				if err != nil {
					output.Error(err)
					continue
				}

				dirName := filepath.Join(home, ".config", "dctl", "infra", name)
				err = os.RemoveAll(dirName)
				if err != nil {
					output.Error(err)
					continue
				}
			}
		}

	}

	return nil
}
