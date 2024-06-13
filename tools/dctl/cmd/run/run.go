package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	e "github.com/steady-bytes/tools/dctl/execute"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	Services []string
	Domains  []string
)

func Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if len(Services) > 0 && len(Domains) > 0 {
		return fmt.Errorf("cannot specify both services and domains to run at once")
	}

	if len(Services) == 0 && len(Domains) == 0 {
		return fmt.Errorf("must specify at least one service or one domain to run")
	}

	if len(Services) > 0 {
		output.Println("running service(s): %s", Services)

		rootPath := viper.GetString("config.root")
		for _, name := range Services {
			paths := strings.Split(name, string(os.PathSeparator))
			if len(paths) != 2 {
				return fmt.Errorf("invalid service name, must take shape 'domain/service': %s", name)
			}
			go run(ctx, filepath.Join(rootPath, "services", paths[0]), paths[1])
		}
	}

	if len(Domains) > 0 {
		output.Println("running domain(s): %s", Domains)

		rootPath := viper.GetString("config.root")
		for _, d := range Domains {
			// build out execution path
			domainPath := filepath.Join(rootPath, "services", d)

			// iterate through all services in domain
			services, err := os.ReadDir(domainPath)
			if err != nil {
				return err
			}
			for _, s := range services {
				if s.IsDir() {
					go run(ctx, domainPath, s.Name())
				}
			}
		}
	}

	// wait for user cancel
	<-ctx.Done()

	return nil
}

func run(ctx context.Context, path, name string) {
	c := exec.Command("go", "run", "main.go")
	c.Dir = filepath.Join(path, name)
	err := e.ExecuteCommand(ctx, name, output.Blue, c)
	if err != nil {
		output.Error(err)
	}
}
