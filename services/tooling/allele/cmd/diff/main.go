// Command diff is Phase 5's internal diagnostic tool, per
// docs/website/content/docs/architecture/allele-implementation-plan.md: compute the symbol-level
// diff between two refs of a real git repository and print it, proving the diff core works against
// real repository history before AlleleService's own GetChange has anything persisted (a Change
// only exists once Phase 7 gives worktrees a real row to back it) to call it against. Not part of
// the running service -- a standalone binary for exactly this one check, the same role cmd/parse
// played for Phase 4's grammar integration.
//
// Usage: go run ./cmd/diff <repo-path> <base-ref> <compare-ref>
package main

import (
	"fmt"
	"os"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/diff"
	"github.com/steady-bytes/draft/services/tooling/allele/gitutil"
	"github.com/steady-bytes/draft/services/tooling/allele/parsing"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: diff <repo-path> <base-ref> <compare-ref>")
		os.Exit(1)
	}
	repoPath, baseRef, compareRef := os.Args[1], os.Args[2], os.Args[3]

	paths, err := gitutil.ChangedFiles(repoPath, baseRef, compareRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing changed files: %v\n", err)
		os.Exit(1)
	}
	if len(paths) == 0 {
		fmt.Println("no changed files")
		return
	}

	total := 0
	for _, path := range paths {
		lang := parsing.LanguageForPath(path)
		if lang == parsing.LanguageUnknown {
			fmt.Printf("%s: skipped (no grammar for this extension)\n", path)
			continue
		}

		oldSrc, err := gitutil.ShowFile(repoPath, baseRef, path) // empty, nil error if newly added
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading %s at %s: %v\n", path, baseRef, err)
			os.Exit(1)
		}
		newSrc, err := gitutil.ShowFile(repoPath, compareRef, path) // empty, nil error if deleted
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading %s at %s: %v\n", path, compareRef, err)
			os.Exit(1)
		}

		changes, err := diff.SymbolChanges(lang, path, oldSrc, newSrc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "diffing %s: %v\n", path, err)
			os.Exit(1)
		}
		if len(changes) == 0 {
			fmt.Printf("%s: no symbol-level changes (non-symbol lines only, e.g. comments/imports)\n", path)
			continue
		}
		for _, c := range changes {
			total++
			switch c.GetOp() {
			case allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_SIGNATURE_CHANGED:
				fmt.Printf("%s  ±  %-30s %s\n    was: %s\n    now: %s\n",
					path, c.GetSymbol(), c.GetKind(), c.GetSignatureBefore(), c.GetSignatureAfter())
			case allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_ADDED:
				fmt.Printf("%s  +  %-30s %s\n", path, c.GetSymbol(), c.GetKind())
			case allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_DELETED:
				fmt.Printf("%s  -  %-30s %s\n", path, c.GetSymbol(), c.GetKind())
			default: // MODIFIED
				fmt.Printf("%s  ~  %-30s %s\n", path, c.GetSymbol(), c.GetKind())
			}
		}
	}
	fmt.Printf("\n%d symbol change(s) across %d file(s)\n", total, len(paths))
}
