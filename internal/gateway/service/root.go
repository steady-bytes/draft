package service

import (
	"fmt"
	"os"

	draft "github.com/steady-bytes/draft/pkg/draft-runtime-golang"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	configFile string
	Runtime    *draft.Runtime

	name string
	port int32
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is config.yaml")

	rootCmd.AddCommand(eventStore)
	eventStore.Flags().Int32VarP(&port, "port", "p", 3001, "rpc port override, by default the rpc port is 3001")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gateway",
	Short: "gateway is the entry point to the draft system",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Usage()
	},
}

var eventStore = &cobra.Command{
	Use:   "run",
	Short: "run the draft gateway",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := Runtime.DefaultBuilder(
			NewService(),
		).Start(); err != nil {
			panic(err)
		}

		return nil
	},
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

	name = "gateway"

	cfg := draft.NewConfig(name, port)

	rt, err := draft.New(cfg)
	if err != nil {
		fmt.Printf("failed to start: %v\n", err)
	}

	Runtime = rt
}
