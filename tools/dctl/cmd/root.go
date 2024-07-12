package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dctl",
	Short: "dctl (Draft Controller) is the built-in CLI for managing everything in Draft",
	Long: `dctl (Draft Controller) is the built-in CLI for managing everything in Draft.
It does everything from generate code from Protobufs to spin up your local infrastructure for
development.`,
	DisableAutoGenTag: true,
	SilenceUsage:      true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx := context.Background()

	// trap Ctrl+C and call cancel on the context
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		output.PrintlnWithNameAndColor("dctl", "Failed", output.Red)
		os.Exit(1)
	}
	output.PrintlnWithNameAndColor("dctl", "Finished", output.Green)
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
	rootCmd.PersistentFlags().StringVar(&config.ContextOverride, "context", "", "override the current context")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		if err != nil {
			output.Error(err)
			os.Exit(1)
		}
		cfgFile = filepath.Join(home, ".config", "dctl", "config.yaml")

		// Search config in home directory with name ".dctl" (without extension).
		viper.AddConfigPath(filepath.Join(home, ".config", "dctl"))
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// attempt to read config
	err := viper.ReadInConfig()
	if err != nil {
		output.Warn(err.Error())
		// create config file if not found
		if strings.Contains(err.Error(), "Not Found") {
			output.Print("Creating config file")
			viper.WriteConfigAs(cfgFile)
		}
	} else {
		output.Print("Using config file: %s", viper.ConfigFileUsed())
	}
}

// requireWorkspace can be used as a PreRunE on a cobra.Command to make sure
// the current context is a workspace and fail out if not.
func requireWorkspace(cmd *cobra.Command, args []string) error {
	dctx := config.GetContext()
	if !dctx.IsWorkspace {
		return fmt.Errorf("this command must be called using a context with an associated workspace - make sure the context has a `root` value of a directory with a valid draft.yaml workspace definition file")
	}
	config.SetContext(dctx)
	return nil
}
