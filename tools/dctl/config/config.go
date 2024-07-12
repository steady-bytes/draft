package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/steady-bytes/draft/tools/dctl/output"
)

type (
	Config struct {
		Current  string
		Contexts map[string]Context
	}
	Context struct {
		Repo        string
		Root        string
		TrunkBranch string `mapstructure:"trunk_branch"`
		API         API `mapstructure:"api"`
	}
	API struct {
		ImageName     string `mapstructure:"image_name"`
		ContainerName string `mapstructure:"container_name"`
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

func CurrentContext() Context {
	c := Get()
	return c.Contexts[c.Current]
}

func SetContext(new string) error {
	c := Get()
	_, ok := c.Contexts[new]
	if !ok {
		keys := make([]string, len(c.Contexts))
		i := 0
		for k := range c.Contexts {
			keys[i] = k
			i++
		}
		output.Println("The requested context doesn't exist in the config.")
		output.Println("Available options are: %v", keys)
		return fmt.Errorf("invalid context name")
	}
	viper.Set("current", new)
	return viper.WriteConfig()
}

func Repo() string {
	c := Get()
	return Get().Contexts[c.Current].Repo
}

func Root() string {
	c := Get()
	return Get().Contexts[c.Current].Root
}
