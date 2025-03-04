package context

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/input"
	"github.com/steady-bytes/draft/tools/dctl/output"

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
	f := config.FindWorkspaceFile(path)
	if f == "" {
		return errors.New("no draft context found at the given path or in any parent directory")
	}

	output.Print("Import context from %s? (YES/no)", f)
	if !input.ConfirmDefaultAllow() {
		return nil
	}
	dctx := config.LoadWorkspaceContext(f)

	// check if a default is set and if not set the new context to the default
	d := viper.GetString("default")
	if d == "" {
		viper.SetDefault("default", dctx.Name)
	}

	// save context (with root path) to dctl config
	viper.Set(fmt.Sprintf("contexts.%s.root", dctx.Name), path)
	err = viper.WriteConfig()
	if err != nil {
		return err
	}

	return nil
}
