// Package parsing is Allele's tree-sitter integration -- the "parsing is a reused dependency"
// half of the plan's "AST-level structural diff and 3-way merge" section
// (docs/website/content/docs/architecture/allele-implementation-plan.md). It wraps
// github.com/tree-sitter/go-tree-sitter (the tree-sitter project's own Go bindings) plus one
// grammar per supported language, and is deliberately the *only* thing this package does: picking
// a grammar by file extension and producing a parsed tree. The structural diff/merge core the
// plan calls new engineering (Phases 5-6) is not here -- this package has no notion of a "change,"
// a "conflict," or a second tree to compare against.
package parsing

import (
	"fmt"
	"path/filepath"

	tree_sitter_proto "github.com/coder3101/tree-sitter-proto/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

// Language identifies which tree-sitter grammar governs a file.
type Language int

const (
	LanguageUnknown Language = iota
	LanguageGo
	LanguageRust
	// LanguageProto: the plan disclosed a possible text-diff fallback for .proto specifically, if
	// no tree-sitter grammar held up against a real file (§8.4's three Phase 0 grammars don't
	// include an equally mature community proto grammar the way Go/Rust do). Phase 4 vetted
	// coder3101/tree-sitter-proto against this repo's own real models.proto/service.proto: zero
	// ERROR/MISSING nodes, and real semantic node kinds (message/message_body/enum/enum_field/
	// field/field_number/option), not just an absence of catastrophic failure. The fallback isn't
	// needed -- .proto gets full structural support alongside Go and Rust.
	LanguageProto
)

func (l Language) String() string {
	switch l {
	case LanguageGo:
		return "go"
	case LanguageRust:
		return "rust"
	case LanguageProto:
		return "proto"
	default:
		return "unknown"
	}
}

// LanguageForPath picks a Language by file extension. Returns LanguageUnknown for anything else --
// callers decide what that means (e.g. a text-only fallback), this package never does.
func LanguageForPath(path string) Language {
	switch filepath.Ext(path) {
	case ".go":
		return LanguageGo
	case ".rs":
		return LanguageRust
	case ".proto":
		return LanguageProto
	default:
		return LanguageUnknown
	}
}

func languageFor(l Language) (*tree_sitter.Language, error) {
	switch l {
	case LanguageGo:
		return tree_sitter.NewLanguage(tree_sitter_go.Language()), nil
	case LanguageRust:
		return tree_sitter.NewLanguage(tree_sitter_rust.Language()), nil
	case LanguageProto:
		return tree_sitter.NewLanguage(tree_sitter_proto.Language()), nil
	default:
		return nil, fmt.Errorf("no tree-sitter grammar for language %s", l)
	}
}

// Parse parses src using the grammar for l. The caller owns the returned tree and must call
// tree.Close() when done with it -- go-tree-sitter's own API, not something this package wraps
// further, so the rest of Allele that eventually holds onto a *tree_sitter.Tree (Phase 5's diff
// core) deals with the real type directly rather than through an extra layer.
func Parse(l Language, src []byte) (*tree_sitter.Tree, error) {
	lang, err := languageFor(l)
	if err != nil {
		return nil, err
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("setting language %s: %w", l, err)
	}

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("parsing as %s produced no tree", l)
	}
	return tree, nil
}
