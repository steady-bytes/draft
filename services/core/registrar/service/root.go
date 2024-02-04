package service

import (
	draft "github.com/steady-bytes/draft/pkg/chassis"

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
	Use:   "draft",
	Short: "draft a really cool way to build distributed systems",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Usage()
	},
}

var registrar = &cobra.Command{
	Use:   "registrar",
	Short: "run the registrar component of `draft`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name = "registrar"

		if err := Runtime.DefaultBuilder(rg.NewPlugin()).Start(); err != nil {
			panic(err)
		}

		return nil
	},
}

var (
	configFile string
	Runtime    *draft.Commet

	name     string
	rpcPort  int32
	httpPort int32
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is config.yaml")

	rootCmd.AddCommand(registrar)
	registrar.Flags().Int32VarP(&rpcPort, "rpc_port", "r", 50000, "rpc port override, by default the rpc port is 50002")
	registrar.Flags().Int32VarP(&httpPort, "http_port", "p", 40000, "http port override, by default the http port is 40002")
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

	cfg := draft.NewConfig(name, rpcPort, httpPort)

	rt, err := draft.New(cfg)
	if err != nil {
		fmt.Printf("failed to start: %v\n", err)
	}

	Runtime = rt
}
