package merge

import (
	"fmt"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/diff"
	"github.com/steady-bytes/draft/services/tooling/allele/gitutil"
	"github.com/steady-bytes/draft/services/tooling/allele/parsing"
)

// SideSymbols holds one branch's symbols relative to a shared base, across every file it touches
// -- both the same-node check (ReconcileFunction/ReconcileType) and the broken-reference check
// need the base/changed pair for any symbol either side's own diff flagged, so this is computed
// once per branch and reused for however many pairwise comparisons a caller needs (the overlap
// matrix computes this once per open worktree, not once per pair -- O(n) parses feeding an O(n²)
// comparison, not O(n²) parses).
type SideSymbols struct {
	ID             string                            // the worktree/branch id this side represents, for conflict reports
	BaseSymbols    map[string]map[string]diff.Symbol // path -> name -> Symbol, as of the shared base
	ChangedSymbols map[string]map[string]diff.Symbol // path -> name -> Symbol, as of this side's own ref
	Changes        map[string][]*allelev1.SymbolChange
}

// LoadSideSymbols parses every file ref changes relative to baseRef and returns the resulting
// SideSymbols. id is this side's own identifier (a worktree id), stamped onto any ConflictReport
// Overlap later produces from it.
func LoadSideSymbols(repoPath, baseRef, ref, id string) (*SideSymbols, error) {
	paths, err := gitutil.ChangedFiles(repoPath, baseRef, ref)
	if err != nil {
		return nil, fmt.Errorf("listing %s's changed files: %w", id, err)
	}
	s := &SideSymbols{
		ID:             id,
		BaseSymbols:    make(map[string]map[string]diff.Symbol),
		ChangedSymbols: make(map[string]map[string]diff.Symbol),
		Changes:        make(map[string][]*allelev1.SymbolChange),
	}
	for _, path := range paths {
		lang := parsing.LanguageForPath(path)
		if lang == parsing.LanguageUnknown {
			continue
		}
		baseSrc, err := gitutil.ShowFile(repoPath, baseRef, path)
		if err != nil {
			return nil, err
		}
		newSrc, err := gitutil.ShowFile(repoPath, ref, path)
		if err != nil {
			return nil, err
		}
		baseByName, err := diff.SymbolsByName(lang, baseSrc)
		if err != nil {
			return nil, fmt.Errorf("%s: parsing base %s: %w", id, path, err)
		}
		newByName, err := diff.SymbolsByName(lang, newSrc)
		if err != nil {
			return nil, fmt.Errorf("%s: parsing %s: %w", id, path, err)
		}
		changes, err := diff.SymbolChanges(lang, path, baseSrc, newSrc)
		if err != nil {
			return nil, fmt.Errorf("%s: diffing %s: %w", id, path, err)
		}
		s.BaseSymbols[path] = baseByName
		s.ChangedSymbols[path] = newByName
		s.Changes[path] = changes
	}
	return s, nil
}

func touchedNames(changes []*allelev1.SymbolChange) map[string]bool {
	out := make(map[string]bool, len(changes))
	for _, c := range changes {
		out[c.GetSymbol()] = true
	}
	return out
}

// OverlapResult is the full R2.1/R2.2 relationship between two independently-diverged sides
// sharing the same base. SharedFiles alone (with AutoMerged/Conflicts both empty) is the mockup's
// own "same file, different symbols · merges cleanly" case -- genuinely distinct from AutoMerged,
// which only names a symbol *both sides actually touched* and reconciled; two sides touching
// completely disjoint symbols in the same file never enters that path at all, so without tracking
// shared files directly, that case would be indistinguishable from no overlap whatsoever. Conflicts
// holds every real clash, same-node or broken-reference, each with its own structural explanation
// (R2.3).
type OverlapResult struct {
	SharedFiles []string
	AutoMerged  []string
	Conflicts   []*allelev1.ConflictReport
}

// Overlap computes OverlapResult for a pair of SideSymbols. b's own id is stamped as the
// "other_worktree_id" on every ConflictReport produced -- call it the other way too
// (Overlap(b, a)) for the symmetric report naming a as the other side.
func Overlap(a, b *SideSymbols) (OverlapResult, error) {
	var result OverlapResult

	for path := range a.ChangedSymbols {
		if _, shared := b.ChangedSymbols[path]; shared {
			result.SharedFiles = append(result.SharedFiles, path)
		}
	}

	// Same-node (R2.1/R2.2): files changed on both sides, symbols both sides' own diffs actually
	// flagged as touched.
	for path, aByName := range a.ChangedSymbols {
		bByName, filesOverlap := b.ChangedSymbols[path]
		if !filesOverlap {
			continue
		}
		aTouched, bTouched := touchedNames(a.Changes[path]), touchedNames(b.Changes[path])
		for name := range aTouched {
			if !bTouched[name] {
				continue
			}
			aSym, inA := aByName[name]
			bSym, inB := bByName[name]
			if !inA || !inB {
				continue // one side deleted it entirely -- not this check's case
			}

			baseSym, inBase := a.BaseSymbols[path][name]
			if !inBase {
				if aSym.Text == bSym.Text {
					result.AutoMerged = append(result.AutoMerged, name)
				} else {
					result.Conflicts = append(result.Conflicts, &allelev1.ConflictReport{
						Id: name + "#add-add", Kind: allelev1.ConflictKind_CONFLICT_KIND_SAME_NODE,
						Symbol: name, OtherWorktreeId: b.ID, OtherSymbol: name,
						Explanation: fmt.Sprintf("both sides independently added %s with different content", name),
					})
				}
				continue
			}

			var rec SameNodeResult
			var err error
			switch aSym.Kind {
			case "function", "method":
				rec, err = ReconcileFunction(baseSym, aSym, bSym, b.ID)
			case "type":
				rec, err = ReconcileType(baseSym, aSym, bSym, b.ID)
			default:
				continue // struct/enum/message aren't Go-decomposable the same way yet -- see Decisions
			}
			if err != nil {
				return result, fmt.Errorf("reconciling %s %s: %w", path, name, err)
			}
			if rec.Mergeable {
				result.AutoMerged = append(result.AutoMerged, name)
			} else {
				result.Conflicts = append(result.Conflicts, rec.Conflict)
			}
		}
	}

	// Broken reference (R2.2, cross-file), both directions: a removing a field b's changed code
	// now reads, and b removing a field a's changed code now reads.
	result.Conflicts = append(result.Conflicts, findBrokenReferences(a, b)...)
	result.Conflicts = append(result.Conflicts, findBrokenReferences(b, a)...)

	return result, nil
}

// findBrokenReferences checks remover's own changed type symbols for field removals, then checks
// referencer's own changed functions/methods, across every file, for a reference to a removed
// name -- scanning only each side's changed surface, not the whole repository (see
// BrokenReference's own doc comment for the full, deliberate scope statement).
func findBrokenReferences(remover, referencer *SideSymbols) []*allelev1.ConflictReport {
	var out []*allelev1.ConflictReport
	for path, byName := range remover.ChangedSymbols {
		removerTouched := touchedNames(remover.Changes[path])
		for name, sym := range byName {
			if sym.Kind != "type" || !removerTouched[name] {
				continue
			}
			baseSym, ok := remover.BaseSymbols[path][name]
			if !ok {
				continue
			}
			removedFields, structOK, err := RemovedStructFields(baseSym, sym)
			if err != nil || !structOK || len(removedFields) == 0 {
				continue
			}

			for refPath, refByName := range referencer.ChangedSymbols {
				refTouched := touchedNames(referencer.Changes[refPath])
				for refName, refSym := range refByName {
					if (refSym.Kind != "function" && refSym.Kind != "method") || !refTouched[refName] {
						continue
					}
					refBase := referencer.BaseSymbols[refPath][refName] // zero value if newly added -- fine, see BrokenReference
					report, err := BrokenReference(refBase, refSym, removedFields, name, remover.ID)
					if err != nil || report == nil {
						continue
					}
					out = append(out, report)
				}
			}
		}
	}
	return out
}
