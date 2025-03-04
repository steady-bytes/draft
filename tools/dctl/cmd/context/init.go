package context

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/input"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)
const (
	templateDir = "template"

	errNoInput = "no input provided"
)

var (
	Path string

	//go:embed template
	files embed.FS
)

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	Path, err := filepath.Abs(Path)
	if err != nil {
		return err
	}

	// get name
	output.Print("What is the name of this context?")
	name := input.Get()
	if name == "" {
		return errors.New(errNoInput)
	}

	// get repo
	output.Print("What is the git repository for this context? (e.g. github.com/steady-bytes/draft)")
	repo := input.Get()
	if repo == "" {
		return errors.New(errNoInput)
	}

	// confirm path
	output.Print("This will initialize a new Draft context in the directory: %s", Path)
	output.Print("Would you like to proceed? (yes/NO)")
	if !input.ConfirmDefaultDeny() {
		return nil
	}
	viper.Set(fmt.Sprintf("contexts.%s.root", name), Path)

	output.Print("Intializing context...")

	// make sure path exists
	_, err = os.ReadDir(Path)
	if err != nil {
		return err
	}

	// make all required directories
	err = os.Mkdir(filepath.Join(Path, "api"), os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Mkdir(filepath.Join(Path, "deployments"), os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Mkdir(filepath.Join(Path, "pipelines"), os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Mkdir(filepath.Join(Path, "pkg"), os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Mkdir(filepath.Join(Path, "services"), os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Mkdir(filepath.Join(Path, "tests"), os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Mkdir(filepath.Join(Path, "tools"), os.ModePerm)
	if err != nil {
		return err
	}

	// write files
	err = writeFiles(templateDir)
	if err != nil {
		return err
	}

	// write context file
	dc := config.Context{
		Name: name,
		Repo: repo,
		TrunkBranch: defaultTrunkBranch,
		API: config.API{
			ImageName: defaultAPIImageName,
		},
	}
	out, err := yaml.Marshal(dc)
	if err != nil {
		output.Error(err)
		return errors.New("failed to marshal context file")
	}
	err = os.WriteFile(filepath.Join(Path, "draft.yaml"), out, 0666)
	if err != nil {
		output.Error(err)
		return errors.New("failed to write context file")
	}

	// initialize api go module
	command := exec.Command("go", "mod", "init", fmt.Sprintf("%s/api", repo))
	command.Dir = filepath.Join(Path, "api")
	err = execute.ExecuteCommand(ctx, "go", output.Cyan, command)
	if err != nil {
		return err
	}

	err = viper.WriteConfig()
	if err != nil {
		return err
	}

	return nil
}

func writeFiles(dir string) error {
	entries, err := files.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		readPath := filepath.Join(dir, e.Name())
		writePath := strings.TrimPrefix(readPath, templateDir+string(os.PathSeparator))
		if e.IsDir() {
			err = writeFiles(readPath)
			if err != nil {
				return err
			}
			continue
		}
		f, err := files.ReadFile(readPath)
		if err != nil {
			return err
		}
		err = os.WriteFile(filepath.Join(Path, writePath), f, os.ModePerm)
		if err != nil {
			return err
		}
	}

	return nil
}
