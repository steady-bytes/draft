package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/steady-bytes/draft/tools/dctl/output"
)

var (
	Force bool
)

func Config(cmd *cobra.Command, args []string) error {
	// build config file path
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fileName := filepath.Join(home, ".config", "dctl", "config.yaml")

	v := viper.New()
	v.Set("current", "")

	if Force {
		err = v.WriteConfigAs(fileName)
		if err != nil {
			return err
		}
	} else {
		err = v.SafeWriteConfigAs(fileName)
		if err != nil {
			if strings.Contains(err.Error(), "Already Exists") {
				output.Print("Config file already exists. Provide --force/-f flag to overwrite.")
				return nil
			}
			return err
		}
	}

	return nil
}
