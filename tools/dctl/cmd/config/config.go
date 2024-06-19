package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func Config(cmd *cobra.Command, args []string) error {
	err := createConfigFile()
	if err != nil {
		return err
	}
	return nil
}

func createConfigFile() error {
	// get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// build config file path
	fileName := filepath.Join(home, ".config", "dctl", "config.yaml")

	// create if not exists
	_, err = os.Stat(fileName)
	if os.IsNotExist(err) {

		// make sure the directory exists
		err = os.MkdirAll(filepath.Join(home, ".config", "dctl"), 0755)
		if err != nil {
			return err
		}

		// create the file
		file, err := os.Create(fileName)
		if err != nil {
			return err
		}
		defer file.Close()
	}

	return nil
}
