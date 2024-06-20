package api

import (
	"os"
	"path/filepath"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Clean(cmd *cobra.Command, args []string) (err error) {
	output.Println("Cleaning api...")

	// build out execution path
	rootPath := config.Root()
	apiPath := filepath.Join(rootPath, "api")

	// remove buf.lock
	os.Remove(filepath.Join(apiPath, "buf.lock"))
	// clean go
	output.Println("Cleaning go...")
	err = deleteAllSubDirectories(filepath.Join(apiPath, "gen", "go"))
	if err != nil {
		return err
	}
	// clean web
	output.Println("Cleaning web...")
	err = deleteAllSubDirectories(filepath.Join(apiPath, "gen", "web"))
	if err != nil {
		return err
	}

	return err
}

// deleteAllSubDirectories removes all subdirectories and their children without affecting the top-level non-directory files
func deleteAllSubDirectories(path string) error {
	dir, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, d := range dir {
		if d.IsDir() {
			os.RemoveAll(filepath.Join(path, d.Name()))
		}
	}
	return nil
}
