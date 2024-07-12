package context

import (
	"fmt"
	"os"
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
	Path, err := filepath.Abs(Path)
	if err != nil {
		return err
	}

	// get name
	output.Println("What is the name of this context?")
	name := input.Get()

	// get repo
	output.Println("What is the git repository for this context? (e.g. github.com/steady-bytes/draft)")
	repo := input.Get()
	viper.Set(fmt.Sprintf("contexts.%s.repo", name), repo)

	// set path
	_, err = os.ReadDir(Path)
	if err != nil {
		return err
	}
	viper.Set(fmt.Sprintf("contexts.%s.root", name), Path)

	setDefaults(name)

	// write context to config
	err = viper.WriteConfig()
	if err != nil {
		return err
	}

	// set context
	err = config.SetContext(name)
	if err != nil {
		return err
	}
	output.Println("The current context is now: %s", name)

	return nil
}
