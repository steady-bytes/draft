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

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return err
	}

	output.Print("Creating dedicated Docker network")
	err = dockerCtl.CreateNetwork(ctx, "draft")
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	for _, name := range Services {
		config, err := getInfraConfig(name)
		if err != nil {
			output.Error(err)
			return err
		}
		output.Print("Pulling Docker image for: %s", name)
		err = dockerCtl.PullImage(ctx, config.containerConfig.Image)
		if err != nil {
			return err
		}

		if config.configFile != nil {
			output.Print("Writing configuration file for: %s", name)
			home, err := os.UserHomeDir()
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}

			dirName := filepath.Join(home, ".config", "dctl", "infra")
			err = os.Mkdir(dirName, 0777)
			if err != nil && !os.IsExist(err) {
				output.Error(err)
				os.Exit(1)
			}

			fileName := filepath.Join(dirName, fmt.Sprintf("%s.yaml", name))
			err = os.WriteFile(fileName, []byte(config.configFile.contents), 0777)
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}
		}

		if config.mountPoint != nil {
			output.Print("Initializing volume for: %s", name)
			home, err := os.UserHomeDir()
			if err != nil {
				output.Error(err)
				os.Exit(1)
			}

			dirName := filepath.Join(home, ".config", "dctl", "infra", name)
			err = os.MkdirAll(dirName, 0777)
			if err != nil && !os.IsExist(err) {
				output.Error(err)
				os.Exit(1)
			}
		}
	}

	return nil
}
