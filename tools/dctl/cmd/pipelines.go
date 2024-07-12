package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/pipelines"

	"github.com/spf13/cobra"
)

var pipelinesCmd = &cobra.Command{
	Use:     "pipelines",
	Aliases: []string{"pipeline", "p"},
	Short:   "Manage and run draft pipelines",
}

var pipelinesInitCmd = &cobra.Command{
	Use:     "init",
	Short:   "Initialize pipelines configuration",
	PreRunE: requireWorkspace,
	RunE:    pipelines.Init,
}

var pipelinesCleanCmd = &cobra.Command{
	Use:     "clean",
	Short:   "Cleanup any existing pipelines configuration on the cluster",
	PreRunE: requireWorkspace,
	RunE:    pipelines.Clean,
}

var pipelinesDashboardCmd = &cobra.Command{
	Use:     "dashboard",
	Aliases: []string{"dash"},
	Short:   "Start the dashboard",
	RunE:    pipelines.Dashboard,
}

var pipelinesRunCmd = &cobra.Command{
	Use:     "run",
	Short:   "Run a pipeline",
	PreRunE: requireWorkspace,
	RunE:    pipelines.Run,
}

func init() {
	// add parent
	rootCmd.AddCommand(pipelinesCmd)
	// add children
	pipelinesCmd.AddCommand(pipelinesInitCmd)
	pipelinesInitCmd.Flags().StringVarP(&pipelines.SshIdFile, "file", "f", "", "the file containing your ssh private key")
	pipelinesCmd.AddCommand(pipelinesCleanCmd)
	pipelinesCmd.AddCommand(pipelinesDashboardCmd)
	pipelinesDashboardCmd.Flags().StringVarP(&pipelines.DashboardPort, "port", "p", "9097", "the localhost port to forward the dashboard to")
	pipelinesCmd.AddCommand(pipelinesRunCmd)
	pipelinesRunCmd.Flags().StringVarP(&pipelines.RunConfig.Pipeline, "pipeline", "p", "", "the pipeline to run")
	pipelinesRunCmd.Flags().StringVarP(&pipelines.RunConfig.RepoUrl, "repo", "r", "git@github.com:steady-bytes/draft.git", "the git repository url to clone")
	pipelinesRunCmd.Flags().StringVarP(&pipelines.RunConfig.Revision, "revision", "v", "main", "the revision of the repo to clone")
	pipelinesRunCmd.Flags().StringVarP(&pipelines.RunConfig.Directory, "directory", "d", "", "the directory within the repo to test")
}
