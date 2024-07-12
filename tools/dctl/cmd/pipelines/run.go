package pipelines

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

var (
	RunConfig TemplateConfig
)

type TemplateConfig struct {
	Pipeline  string
	RepoUrl   string
	Revision  string
	Directory string
}

func Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctx := config.GetContext()

	p := filepath.Join(dctx.Root, "pipelines", "runs", fmt.Sprintf("%s.yaml", RunConfig.Pipeline))

	// read in go template from def
	def, err := os.ReadFile(p)
	if err != nil {
		return err
	}

	// create template
	t, err := template.New("pipeline").Parse(string(def))
	if err != nil {
		return err
	}

	// create temp file
	f, err := os.CreateTemp("", fmt.Sprintf("dctl-pipeline-%s-*.yaml", RunConfig.Pipeline))
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())

	// execute template
	err = t.Execute(f, RunConfig)
	if err != nil {
		return err
	}

	// run pipeline
	command := exec.Command("kubectl", "create", "-f", f.Name())
	err = execute.ExecuteCommand(ctx, "kubectl", output.Cyan, command)
	if err != nil {
		return err
	}

	return nil
}
