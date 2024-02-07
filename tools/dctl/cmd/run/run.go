package run

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/steady-bytes/tools/dctl/output"

	e "github.com/steady-bytes/tools/dctl/execute"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	Domain  string
	Service string
)

func Run(cmd *cobra.Command, args []string) error {

	output.Println("running service:")
	output.Println("- domain: %s", Domain)
	output.Println("- service: %s", Service)

	ctx := cmd.Context()

	// build out execution path
	servicePath := fmt.Sprintf("services/%s/%s", Domain, Service)
	rootPath := viper.GetString("config.root")
	fullPath := filepath.Join(rootPath, servicePath)

	// configure command
	c := exec.Command("go", "run", "main.go")
	c.Dir = fullPath

	// execute command
	err := e.ExecuteCommand(ctx, Service, output.Blue, c)
	if err != nil {
		output.Error(err)
	}

	return err
}
