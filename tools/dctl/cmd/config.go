package cmd

import (
	"os"
	"path/filepath"

	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCommand = &cobra.Command{
	Use:   "config",
	Short: "Initialize the dctl configuration",
	Long: `Initialize the dctl configuration. Be sure to run
this from the root of the draft repository`,
	RunE: func(cmd *cobra.Command, args []string) error {

		err := createConfigFile()
		if err != nil {
			return err
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		output.Println("Setting the root directory to: %s", cwd)
		viper.Set("config.root", cwd)
		err = viper.WriteConfig()
		if err != nil {
			return err
		}
		return nil
	},
}

// HELPER FUNCTIONS

// createConfigFile is a helper function to create the config file if it does not already exist
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

func init() {
	// add parent
	rootCmd.AddCommand(configCommand)
}
