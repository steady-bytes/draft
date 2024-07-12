package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/run"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run services locally",
	Long: `Run services locally. You can either run specific services using the --services (-s)
flag or run entire domains using the --domains (-d) flag.

For example, to run just the 'echo' service within the 'examples' domain you could do:

dctl run -s examples/echo

And to run all services within both the 'examples' domain and the 'core' domain you could do:

dctl run -d examples,core
`,
	PreRunE: requireWorkspace,
	RunE:    run.Run,
}

func init() {
	// add parent
	rootCmd.AddCommand(runCmd)
	// add children
	runCmd.Flags().StringSliceVarP(&run.Services, "services", "s", []string{}, "service(s) to run (e.g. 'core/blueprint' or 'core/blueprint,core/fuse')")
	runCmd.Flags().StringSliceVarP(&run.Domains, "domains", "d", []string{}, "domain(s) to run (e.g. 'core' or 'core,examples')")
}
