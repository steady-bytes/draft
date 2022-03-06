package cmd

import (
	"commet"

	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "firegraph",
	Short: "firegraph is a graph database that kick's ass",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Usage()
	},
}

var (
	configFile string
	Runtime    *commet.Commet

	port int32
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is config.yaml")
}

func initConfig() {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("unable to read config: %v\n", err)
		os.Exit(1)
	}

	cfg := commet.NewConfig()

	rt, err := commet.New(cfg)
	if err != nil {
		fmt.Printf("failed to start: %v\n", err)
	}

	Runtime = rt
}
