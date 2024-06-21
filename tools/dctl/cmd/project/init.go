package project

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steady-bytes/draft/tools/dctl/input"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const templateDir = "template"

var (
	Path string

	//go:embed template
	files embed.FS
)

func Init(cmd *cobra.Command, args []string) error {

	Path, err := filepath.Abs(Path)
	if err != nil {
		return err
	}

	// get name
	output.Println("What is the name of this project?")
	name := input.Get()

	// get repo
	output.Println("What is the git repository for this project? (e.g. github.com/steady-bytes/draft)")
	repo := input.Get()
	viper.Set(fmt.Sprintf("projects.%s.repo", name), repo)

	// confirm path
	output.Println("This will initialize a new Draft project in the directory: %s", Path)
	output.Println("Would you like to proceed? (yes/NO)")
	if !input.ConfirmDefaultDeny() {
		return nil
	}
	viper.Set(fmt.Sprintf("projects.%s.root", name), Path)

	output.Println("Intializing project...")

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

	setDefaults(name)

	// write project to config
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
		writePath := strings.TrimPrefix(readPath, templateDir + string(os.PathSeparator))
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

func setDefaults(name string) {
	viper.Set(fmt.Sprintf("projects.%s.api.image_name", name), defaultAPIImageName)
	viper.Set(fmt.Sprintf("projects.%s.api.container_name", name), defaultAPIContainerName)
	viper.Set(fmt.Sprintf("projects.%s.trunk_branch", name), defaultTrunkBranch)
}
