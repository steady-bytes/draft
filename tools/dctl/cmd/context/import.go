package context

import (
	"fmt"
	"path/filepath"

	"github.com/steady-bytes/draft/tools/dctl/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultAPIImageName     = "ghcr.io/steady-bytes/draft-api-builder:main"
	defaultAPIContainerName = "draft-api-builder"
	defaultTrunkBranch      = "main"
)

func Import(cmd *cobra.Command, args []string) error {
	// load the workspace file from the given path
	path, err := filepath.Abs(Path)
	if err != nil {
		return err
	}
	dctx := config.LoadWorkspaceContext(filepath.Join(path, "draft.yaml"))

	// check if a default is set and if not set the new context to the default
	c := config.Get()
	if c.Default == "" {
		config.SetDefaultContext(dctx.Name)
	}

	// save context (with root path) to dctl config
	viper.Set(fmt.Sprintf("contexts.%s.root", dctx.Name), path)
	err = viper.WriteConfig()
	if err != nil {
		return err
	}

	return nil
}
