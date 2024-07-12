package pipelines

import (
	"fmt"
	"os/exec"

	"github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

var (
	DashboardPort string
)

func Dashboard(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	output.Print("Starting dashboard at: http://localhost:%s", DashboardPort)
	command := exec.Command("kubectl", "--namespace", "tekton-pipelines", "port-forward", "svc/tekton-dashboard", fmt.Sprintf("%s:9097", DashboardPort))
	err := execute.ExecuteCommand(ctx, "kubectl", output.Cyan, command)
	if err != nil {
		return err
	}

	return nil
}
