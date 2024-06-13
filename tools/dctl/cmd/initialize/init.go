package initialize

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/steady-bytes/tools/dctl/input"
	"github.com/steady-bytes/tools/dctl/output"

	"github.com/spf13/cobra"
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

	output.Println("This will initialize a new Draft project in the directory: %s", Path)
	output.Println("Would you like to proceed? (yes/NO)")
	if !input.ConfirmDefaultDeny() {
		return nil
	}
	output.Println("Intializing project...")

	// make sure path exists
	_, err = os.ReadDir(Path)
	if err != nil {
		return err
	}

	// make all required directories
	os.Mkdir(filepath.Join(Path, "api"), os.ModePerm)
	os.Mkdir(filepath.Join(Path, "deployments"), os.ModePerm)
	os.Mkdir(filepath.Join(Path, "pipelines"), os.ModePerm)
	os.Mkdir(filepath.Join(Path, "pkg"), os.ModePerm)
	os.Mkdir(filepath.Join(Path, "services"), os.ModePerm)
	os.Mkdir(filepath.Join(Path, "tests"), os.ModePerm)
	os.Mkdir(filepath.Join(Path, "tools"), os.ModePerm)

	err = writeFiles(templateDir)
	if err != nil {
		return err
	}

	return err
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
