// Command parse is Phase 4's internal diagnostic tool, per
// docs/website/content/docs/architecture/allele-implementation-plan.md's Implementation Plan:
// parse a real file and print its tree-sitter syntax tree, proving the grammar integration works
// end to end before any diffing logic exists. Not part of the running service -- a standalone
// binary for exactly this one check, the same role Relay's own cmd/list-devices played for
// verifying its PortAudio integration independent of the full service.
//
// Usage: go run ./cmd/parse <file>
package main

import (
	"fmt"
	"os"

	"github.com/steady-bytes/draft/services/tooling/allele/parsing"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: parse <file>")
		os.Exit(1)
	}
	path := os.Args[1]

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading %s: %v\n", path, err)
		os.Exit(1)
	}

	lang := parsing.LanguageForPath(path)
	if lang == parsing.LanguageUnknown {
		fmt.Fprintf(os.Stderr, "no grammar for %s (supported: .go, .rs, .proto)\n", path)
		os.Exit(1)
	}

	tree, err := parsing.Parse(lang, src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing %s as %s: %v\n", path, lang, err)
		os.Exit(1)
	}
	defer tree.Close()

	root := tree.RootNode()
	fmt.Printf("file:     %s\n", path)
	fmt.Printf("language: %s\n", lang)
	fmt.Printf("root:     %s (%d top-level children, byte range [%d,%d))\n",
		root.Kind(), root.ChildCount(), root.StartByte(), root.EndByte())
	if root.HasError() {
		fmt.Println("WARNING: tree contains at least one parse error node")
	}
	fmt.Println()
	fmt.Println(root.ToSexp())
}
