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
		Projects map[string]Project
	}
	Project struct {
		Repo string
		Root string
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

func CurrentProject() Project {
	c := Get()
	return c.Projects[c.Current]
}

func SetProject(new string) error {
	c := Get()
	_, ok := c.Projects[new]
	if !ok {
		keys := make([]string, len(c.Projects))
		i := 0
		for k := range c.Projects {
			keys[i] = k
			i++
		}
		output.Println("The requested project doesn't exist in the config.")
		output.Println("Available options are: %v", keys)
		return fmt.Errorf("invalid project name")
	}
	viper.Set("current", new)
	return viper.WriteConfig()
}

func Repo() string {
	c := Get()
	return Get().Projects[c.Current].Repo
}

func Root() string {
	c := Get()
	return Get().Projects[c.Current].Root
}
