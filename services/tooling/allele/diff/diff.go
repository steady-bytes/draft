// Package diff is Phase 5 of the plan's Implementation Plan: two-way structural diff +
// SymbolChange extraction. It matches symbols between an old and new version of one file by name
// and classifies each as added, deleted, modified, or signature-changed.
//
// Deliberately NOT here: generic, node-level GumTree-style matching (disjoint-sub-field
// reasoning, move detection). Nothing at symbol-change granularity needs it -- that's genuinely
// Phase 6's job (three-way merge + conflict detection), where it's required to decide whether two
// sides' edits to the *same* symbol are disjoint or clash. Building it now, ahead of Phase 6's own
// concrete requirements, would mean guessing at a shape Phase 6 would likely have to revise anyway
// -- the same "don't build ahead of need" discipline this doc set already applies elsewhere.
package diff

import (
	"fmt"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/parsing"
)

// SymbolChanges computes the symbol-level diff between oldSrc and newSrc of the same file (path is
// stamped onto every result, purely for display/identity -- this function has no notion of a repo
// or a commit). A symbol present in both with byte-identical text is unchanged and omitted
// entirely, matching the mockup's own change view, which never lists an untouched symbol.
func SymbolChanges(lang parsing.Language, path string, oldSrc, newSrc []byte) ([]*allelev1.SymbolChange, error) {
	oldSymbols, err := symbolsFor(lang, oldSrc)
	if err != nil {
		return nil, fmt.Errorf("parsing old version of %s: %w", path, err)
	}
	newSymbols, err := symbolsFor(lang, newSrc)
	if err != nil {
		return nil, fmt.Errorf("parsing new version of %s: %w", path, err)
	}

	oldByName := make(map[string]Symbol, len(oldSymbols))
	for _, s := range oldSymbols {
		oldByName[s.Name] = s
	}
	newByName := make(map[string]Symbol, len(newSymbols))
	for _, s := range newSymbols {
		newByName[s.Name] = s
	}

	var changes []*allelev1.SymbolChange

	// Deleted, modified, and signature-changed: walk old symbols in their own declaration order,
	// so anything that didn't move keeps a stable, predictable output order.
	for _, s := range oldSymbols {
		newSym, stillExists := newByName[s.Name]
		if !stillExists {
			changes = append(changes, &allelev1.SymbolChange{
				Id:     path + "#" + s.Name,
				Path:   path,
				Symbol: s.Name,
				Kind:   s.Kind,
				Op:     allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_DELETED,
			})
			continue
		}
		if change := compareSymbol(path, s, newSym); change != nil {
			changes = append(changes, change)
		}
	}

	// Added: anything in new not present in old, in the new file's own declaration order.
	for _, s := range newSymbols {
		if _, existed := oldByName[s.Name]; !existed {
			changes = append(changes, &allelev1.SymbolChange{
				Id:     path + "#" + s.Name,
				Path:   path,
				Symbol: s.Name,
				Kind:   s.Kind,
				Op:     allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_ADDED,
			})
		}
	}

	return changes, nil
}

// SymbolsByName parses src and returns its top-level symbols keyed by name -- what Phase 6's
// merge package needs (the actual Symbol, including its Text) to reconcile a symbol both sides
// touched, which the plain *allelev1.SymbolChange results from SymbolChanges don't carry.
func SymbolsByName(lang parsing.Language, src []byte) (map[string]Symbol, error) {
	symbols, err := symbolsFor(lang, src)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]Symbol, len(symbols))
	for _, s := range symbols {
		byName[s.Name] = s
	}
	return byName, nil
}

func symbolsFor(lang parsing.Language, src []byte) ([]Symbol, error) {
	tree, err := parsing.Parse(lang, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	return ExtractSymbols(lang, src, tree.RootNode()), nil
}

// compareSymbol decides whether a name-matched pair is unchanged (nil), a signature change, or a
// body-only modification. SIGNATURE_CHANGED fires whenever signature_before != signature_after,
// independent of whether the body also changed -- matching the mockup's own separate "±1 signature
// changed" vs "~6 bodies changed" counts, not folded into one generic MODIFIED bucket. A symbol
// kind with no meaningful signature (type/struct/enum/message) always falls through to MODIFIED.
func compareSymbol(path string, oldSym, newSym Symbol) *allelev1.SymbolChange {
	if oldSym.Text == newSym.Text {
		return nil // byte-identical subtree -- genuinely unchanged
	}

	base := &allelev1.SymbolChange{
		Id:     path + "#" + newSym.Name,
		Path:   path,
		Symbol: newSym.Name,
		Kind:   newSym.Kind,
	}

	if oldSym.Signature != "" && oldSym.Signature != newSym.Signature {
		base.Op = allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_SIGNATURE_CHANGED
		base.SignatureBefore = oldSym.Signature
		base.SignatureAfter = newSym.Signature
		return base
	}

	base.Op = allelev1.SymbolChangeOp_SYMBOL_CHANGE_OP_MODIFIED
	return base
}
