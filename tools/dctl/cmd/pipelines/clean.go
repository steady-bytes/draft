package pipelines

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/input"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

func Clean(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctx := config.GetContext()

	// set the path to the pipelines manifests
	pipelinesPath := filepath.Join(dctx.Root, "pipelines")

	// check current kube context and ask to proceed
	command := exec.Command("kubectl", "config", "current-context")
	kubeContext, err := execute.ExecuteCommandReturnStdout(ctx, command)
	if err != nil {
		return err
	}
	output.Print("Current kube context: %s", kubeContext)
	output.Print("The above context will be used to delete pipeline manifests. Would you like to proceed? (yes/NO)")
	if !input.ConfirmDefaultDeny() {
		output.Warn("Aborted")
		return nil
	}

	// delete all manifests except runs (in reverse order of apply)
	for i := len(manifestPaths) - 1; i >= 0; i-- {
		path := manifestPaths[i]
		if !strings.HasPrefix(path, "https") {
			path = filepath.Join(pipelinesPath, path)
		}
		delete(ctx, path)
	}

	return nil
}

func delete(ctx context.Context, path string) {
	// confirm with user
	output.Print("About to delete the manifest(s) located at: %s", path)
	output.Print("Would you like to proceed? (YES/no)")
	if !input.ConfirmDefaultAllow() {
		output.Warn("Skipped")
		return
	}
	// delete the manifest
	command := exec.Command("kubectl", "delete", "-f", path)
	_ = execute.ExecuteCommand(ctx, "kubectl", output.Cyan, command)
}
