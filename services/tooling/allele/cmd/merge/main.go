// Command merge is Phase 6's internal diagnostic tool, per
// docs/website/content/docs/architecture/allele-implementation-plan.md: given a shared base and
// two independently-diverged refs of a real git repository, reconcile every symbol both sides
// touched (R2.1/R2.2 same-node disjoint-field auto-merge) and check each side's changed code for a
// reference to something the other side's changes take away (R2.2's cross-file broken-reference
// case). A thin CLI wrapper over the merge package's own exported LoadSideSymbols/Overlap -- the
// real orchestration lives there now (Phase 7 moved it out of this file) so AlleleService's own
// GetWorktreeOverlap can call the identical logic, not a parallel copy of it.
//
// Usage: go run ./cmd/merge <repo-path> <base-ref> <ours-ref> <theirs-ref>
package main

import (
	"fmt"
	"os"

	"github.com/steady-bytes/draft/services/tooling/allele/merge"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: merge <repo-path> <base-ref> <ours-ref> <theirs-ref>")
		os.Exit(1)
	}
	repoPath, baseRef, oursRef, theirsRef := os.Args[1], os.Args[2], os.Args[3], os.Args[4]

	ours, err := merge.LoadSideSymbols(repoPath, baseRef, oursRef, "ours")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	theirs, err := merge.LoadSideSymbols(repoPath, baseRef, theirsRef, "theirs")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	result, err := merge.Overlap(ours, theirs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, name := range result.AutoMerged {
		fmt.Printf("auto-merge  %s\n", name)
	}
	fmt.Printf("\n%d symbol(s) auto-merged, %d conflict(s)\n\n", len(result.AutoMerged), len(result.Conflicts))
	for _, c := range result.Conflicts {
		marker := "a line-based merge would conflict here too"
		if c.GetWouldTextuallyMerge() {
			marker = "a line-based merge would succeed"
		}
		fmt.Printf("CONFLICT [%s] %s\n  %s\n  (%s)\n\n", c.GetKind(), c.GetSymbol(), c.GetExplanation(), marker)
	}
}
