package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type (
	Context struct {
		Name        string
		Repo        string
		Root        string
		TrunkBranch string `mapstructure:"trunk_branch" yaml:"trunk_branch"`
		API         API    `mapstructure:"api"  yaml:"api"`
		IsWorkspace bool
	}
	API struct {
		ImageName string `mapstructure:"image_name" yaml:"image_name"`
	}
)

var (
	ContextOverride string
	currentContext  *Context
)

func GetContext() Context {
	// if the current context is already set just use that
	if currentContext != nil {
		return *currentContext
	}

	// load the config
	dconfig := Get()

	// check if the user overrode the context using the global flag
	if ContextOverride != "" {
		return loadConfigContext(dconfig, ContextOverride)
	}

	// check if there is a workspace file in a parent directory
	cwd, err := os.Getwd()
	if err != nil {
		output.Error(err)
		os.Exit(1)
	}
	contextFile := findWorkspaceFile(cwd)
	if contextFile != "" {
		return LoadWorkspaceContext(contextFile)
	}

	// fallback on the default context
	return loadConfigContext(dconfig, dconfig.Default)
}

func SetContext(new Context) {
	currentContext = &new
}

func SetDefaultContext(new string) error {
	dconfig := Get()
	_, ok := dconfig.Contexts[new]
	if !ok {
		invalidContext(dconfig)
		return fmt.Errorf("invalid context name")
	}
	viper.Set("default", new)
	return viper.WriteConfig()
}

func LoadWorkspaceContext(path string) Context {
	output.Print("Using context from workspace file: %s", path)
	f, err := os.ReadFile(path)
	if err != nil {
		output.Error(err)
		os.Exit(1)
	}
	var dctx Context
	err = yaml.Unmarshal(f, &dctx)
	if err != nil {
		output.Error(err)
		os.Exit(1)
	}
	dctx.IsWorkspace = true
	dctx.Root = filepath.Dir(path)
	output.Print("Loaded context: %s", dctx.Name)
	return dctx
}

// modified from the go source code: src/cmd/go/internal/modload/init.go
func findWorkspaceFile(dir string) (root string) {
	if dir == "" {
		output.Error(fmt.Errorf("dir not set"))
		return ""
	}
	dir = filepath.Clean(dir)
	for {
		f := filepath.Join(dir, "draft.yaml")
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
			return f
		}
		d := filepath.Dir(dir)
		if d == dir {
			break
		}
		dir = d
	}
	return ""
}

func loadConfigContext(dconfig Config, contextName string) Context {
	// read the context from the config
	dctx, ok := dconfig.Contexts[contextName]
	if !ok {
		invalidContext(dconfig)
		output.Error(fmt.Errorf("invalid context name"))
		os.Exit(1)
	}

	// check if there is a workspace file defined by the context from the config
	// and load that if available
	if dctx.Root != "" {
		return LoadWorkspaceContext(filepath.Join(dctx.Root, "draft.yaml"))
	}

	// fallback on the context from the config
	output.Print("Loaded context: %s", dctx.Name)
	return dctx
}

func invalidContext(dconfig Config) {
	keys := make([]string, len(dconfig.Contexts))
	i := 0
	for k := range dconfig.Contexts {
		keys[i] = k
		i++
	}
	output.Warn("The requested context doesn't exist in the config.")
	output.Warn("Available options are: %v", keys)
}
