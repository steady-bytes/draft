package merge

import (
	"fmt"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/diff"
	"github.com/steady-bytes/draft/services/tooling/allele/parsing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// RemovedStructFields returns the names of fields present in base but absent in changed, for a
// "type" symbol known to exist (by name) on both sides -- the half of the broken-reference check
// that asks "what did this side take away." ok is false (not an error) if either version isn't a
// plain struct.
func RemovedStructFields(base, changed diff.Symbol) (removed []string, ok bool, err error) {
	baseFields, baseOK, err := StructFields(base.Text)
	if err != nil {
		return nil, false, err
	}
	changedFields, changedOK, err := StructFields(changed.Text)
	if err != nil {
		return nil, false, err
	}
	if !baseOK || !changedOK {
		return nil, false, nil
	}
	changedNames := make(map[string]bool, len(changedFields))
	for _, f := range changedFields {
		changedNames[f.Name] = true
	}
	for _, f := range baseFields {
		if !changedNames[f.Name] {
			removed = append(removed, f.Name)
		}
	}
	return removed, true, nil
}

// ReferencedFields returns the set of field names accessed via a `.field` selector anywhere within
// a Go symbol's own text -- its whole body, recursively, not just top-level children (so a field
// read three statements deep inside a function is found the same as one in its first line).
func ReferencedFields(text string) (map[string]bool, error) {
	tree, err := parsing.Parse(parsing.LanguageGo, []byte(text))
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	src := []byte(text)
	out := make(map[string]bool)
	collectSelectorFields(tree.RootNode(), src, out)
	return out, nil
}

func collectSelectorFields(node *tree_sitter.Node, src []byte, out map[string]bool) {
	if node.Kind() == "selector_expression" {
		if field := node.ChildByFieldName("field"); field != nil {
			out[field.Utf8Text(src)] = true
		}
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		collectSelectorFields(node.NamedChild(i), src, out)
	}
}

// BrokenReference is R2.2's cross-file case: referencingBase/referencingChanged is one side's own
// before/after for a symbol it modified; removedFields names what the *other* side independently
// takes away from some type, by field name. If referencingChanged's own (changed) form reads a
// field the other side removes, that's a real break neither side's own diff alone would show --
// they touch different symbols, often different files, so git's line-based merge sees no conflict
// at all (WouldTextuallyMerge is always true here, the opposite of a SAME_NODE conflict).
//
// This is a deliberate syntactic heuristic, not real type-aware resolution -- stated plainly
// because it's a real, known limitation, not an oversight: it matches by bare field name only,
// never confirming the selector's receiver is actually of the type that lost the field, so a
// same-named field on an unrelated type would false-positive, and a field reached through an
// intermediate variable or alias the AST doesn't make locally obvious would false-negative. It
// exists to catch exactly the PRD's own worked example (and plausible real variants), not as a
// general semantic-conflict engine -- that's explicitly R2.5 (Phase 13, LSP-based: gopls/
// rust-analyzer actually know a selector's receiver type). See the plan's Decisions.
func BrokenReference(referencingBase, referencingChanged diff.Symbol, removedFields []string, removedFromSymbol, otherWorktreeID string) (*allelev1.ConflictReport, error) {
	baseRefs, err := ReferencedFields(referencingBase.Text)
	if err != nil {
		return nil, err
	}
	newRefs, err := ReferencedFields(referencingChanged.Text)
	if err != nil {
		return nil, err
	}

	for _, name := range removedFields {
		if !newRefs[name] {
			continue
		}
		verb := "already reads"
		if !baseRefs[name] {
			verb = "starts reading"
		}
		return &allelev1.ConflictReport{
			Id:                  referencingChanged.Name + "#broken-ref#" + name,
			Kind:                allelev1.ConflictKind_CONFLICT_KIND_BROKEN_REFERENCE,
			Symbol:              referencingChanged.Name,
			OtherWorktreeId:     otherWorktreeID,
			OtherSymbol:         removedFromSymbol,
			Explanation:         fmt.Sprintf("%s removes the field %s.%s that %s %s", otherWorktreeID, removedFromSymbol, name, referencingChanged.Name, verb),
			WouldTextuallyMerge: true,
		}, nil
	}
	return nil, nil
}
