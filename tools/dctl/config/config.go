package config

import (
	"os"

	"github.com/spf13/viper"
	"github.com/steady-bytes/draft/tools/dctl/output"
)

type (
	Config struct {
		Default  string
		Contexts map[string]Context
	}
)

func Get() Config {
	var c Config
	err := viper.Unmarshal(&c)
	if err != nil {
		output.Error(err)
		os.Exit(1)
	}
	return c
}
