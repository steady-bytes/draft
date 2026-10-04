package merge

import (
	"fmt"
	"strings"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"
	"github.com/steady-bytes/draft/services/tooling/allele/diff"
)

// SameNodeResult is the outcome of reconciling both sides' independent changes to one symbol they
// both touched (R2.1/R2.2), relative to their shared base. Conflict is set iff !Mergeable --
// there's nothing to report for a successful auto-merge beyond the fact that it happened; a
// ConflictReport only exists to explain a real clash.
//
// WouldTextuallyMerge is always false for a SAME_NODE conflict here, worth stating explicitly
// since it reads backwards from the broken-reference case: both sides touched the exact same
// line/node, so a plain line-based 3-way merge would *also* flag this as a conflict -- the point
// of this whole check is rescuing the *disjoint* case a textual tool would needlessly flag, not
// claiming textual merge succeeds where it wouldn't.
type SameNodeResult struct {
	Mergeable bool
	Conflict  *allelev1.ConflictReport
}

// ReconcileFunction implements R2.1/R2.2 for a function/method symbol both branches modified: base
// is the shared ancestor's version, ours/theirs are each side's. Disjoint parameter/result slots
// auto-merge (one side's name change plus the other's type change on the *same* parameter); the
// same slot changed differently by both sides is a real conflict, named precisely rather than as
// "lines N-M conflict" (R2.3).
func ReconcileFunction(base, ours, theirs diff.Symbol, theirsWorktreeID string) (SameNodeResult, error) {
	baseParams, baseResult, err := FunctionSlots(base.Text)
	if err != nil {
		return SameNodeResult{}, fmt.Errorf("parsing base %s: %w", base.Name, err)
	}
	oursParams, oursResult, err := FunctionSlots(ours.Text)
	if err != nil {
		return SameNodeResult{}, fmt.Errorf("parsing ours %s: %w", ours.Name, err)
	}
	theirsParams, theirsResult, err := FunctionSlots(theirs.Text)
	if err != nil {
		return SameNodeResult{}, fmt.Errorf("parsing theirs %s: %w", theirs.Name, err)
	}

	n := len(baseParams)
	if len(oursParams) != n || len(theirsParams) != n {
		// Either side added/removed a parameter -- a real arity change on both sides at once.
		// Treated as a conflict rather than attempted as a positional merge across
		// different-length lists: "the Nth parameter" stops being a meaningful shared identity
		// once the lists disagree on length, and guessing which old slot maps to which new one is
		// exactly the kind of semantic judgment call this phase doesn't make (see the plan's
		// Decisions on scope).
		return SameNodeResult{
			Mergeable: false,
			Conflict: &allelev1.ConflictReport{
				Id:                  ours.Name + "#arity",
				Kind:                allelev1.ConflictKind_CONFLICT_KIND_SAME_NODE,
				Symbol:              ours.Name,
				OtherWorktreeId:     theirsWorktreeID,
				OtherSymbol:         theirs.Name,
				Explanation:         fmt.Sprintf("both sides changed %s's parameter count (base had %d; this side has %d, the other has %d) -- can't tell which parameter corresponds to which anymore", ours.Name, n, len(oursParams), len(theirsParams)),
				WouldTextuallyMerge: false,
			},
		}, nil
	}

	var clashes []string
	for i := 0; i < n; i++ {
		if oursParams[i].Name != baseParams[i].Name && theirsParams[i].Name != baseParams[i].Name && oursParams[i].Name != theirsParams[i].Name {
			clashes = append(clashes, fmt.Sprintf("parameter %d's name (%q vs %q)", i+1, oursParams[i].Name, theirsParams[i].Name))
		}
		if oursParams[i].Type != baseParams[i].Type && theirsParams[i].Type != baseParams[i].Type && oursParams[i].Type != theirsParams[i].Type {
			clashes = append(clashes, fmt.Sprintf("parameter %d's type (%q vs %q)", i+1, oursParams[i].Type, theirsParams[i].Type))
		}
	}
	if oursResult != baseResult && theirsResult != baseResult && oursResult != theirsResult {
		clashes = append(clashes, fmt.Sprintf("the result type (%q vs %q)", oursResult, theirsResult))
	}

	if len(clashes) > 0 {
		return SameNodeResult{
			Mergeable: false,
			Conflict: &allelev1.ConflictReport{
				Id:                  ours.Name + "#same-node",
				Kind:                allelev1.ConflictKind_CONFLICT_KIND_SAME_NODE,
				Symbol:              ours.Name,
				OtherWorktreeId:     theirsWorktreeID,
				OtherSymbol:         theirs.Name,
				Explanation:         fmt.Sprintf("both sides changed %s's %s differently", ours.Name, strings.Join(clashes, " and ")),
				WouldTextuallyMerge: false,
			},
		}, nil
	}

	// No clash: every slot either side touched was untouched by the other, or both sides made the
	// identical change to it. Disjoint-field auto-merge, R2.2's own worked example.
	return SameNodeResult{Mergeable: true}, nil
}

// ReconcileType implements the same R2.1/R2.2 reasoning for a struct type's own fields, by name
// (a struct field, unlike a parameter, already has a stable, meaningful identity across versions
// independent of position -- Go field order isn't semantically load-bearing the way parameter
// order is). A field one side removes and the other side retypes is exactly the clash case; two
// sides adding or retyping *different* fields is disjoint and auto-merges.
func ReconcileType(base, ours, theirs diff.Symbol, theirsWorktreeID string) (SameNodeResult, error) {
	baseFields, baseOK, err := StructFields(base.Text)
	if err != nil {
		return SameNodeResult{}, fmt.Errorf("parsing base %s: %w", base.Name, err)
	}
	oursFields, oursOK, err := StructFields(ours.Text)
	if err != nil {
		return SameNodeResult{}, fmt.Errorf("parsing ours %s: %w", ours.Name, err)
	}
	theirsFields, theirsOK, err := StructFields(theirs.Text)
	if err != nil {
		return SameNodeResult{}, fmt.Errorf("parsing theirs %s: %w", theirs.Name, err)
	}
	if !baseOK || !oursOK || !theirsOK {
		// Not a struct on at least one side (interface, alias, ...) -- outside this phase's own
		// worked examples; treat conservatively as a conflict rather than silently approving it.
		return SameNodeResult{
			Mergeable: false,
			Conflict: &allelev1.ConflictReport{
				Id:                  ours.Name + "#non-struct",
				Kind:                allelev1.ConflictKind_CONFLICT_KIND_SAME_NODE,
				Symbol:              ours.Name,
				OtherWorktreeId:     theirsWorktreeID,
				OtherSymbol:         theirs.Name,
				Explanation:         fmt.Sprintf("both sides changed %s, and at least one version isn't a plain struct -- not reconciled structurally, flagged for review", ours.Name),
				WouldTextuallyMerge: false,
			},
		}, nil
	}

	byName := func(fields []NamedTypedSlot) map[string]string {
		m := make(map[string]string, len(fields))
		for _, f := range fields {
			m[f.Name] = f.Type
		}
		return m
	}
	baseM, oursM, theirsM := byName(baseFields), byName(oursFields), byName(theirsFields)

	allNames := make(map[string]struct{})
	for _, m := range []map[string]string{baseM, oursM, theirsM} {
		for name := range m {
			allNames[name] = struct{}{}
		}
	}

	var clashes []string
	for name := range allNames {
		baseType, inBase := baseM[name]
		oursType, inOurs := oursM[name]
		theirsType, inTheirs := theirsM[name]

		oursChanged := inBase != inOurs || baseType != oursType
		theirsChanged := inBase != inTheirs || baseType != theirsType
		if !oursChanged || !theirsChanged {
			continue // only one side touched this field (or neither) -- no clash, not reported
		}
		// Both sides changed the same named field. Disjoint only if they made the *identical*
		// change (e.g. both removed it, or both retyped it the same way); anything else clashes --
		// unlike a parameter, a struct field has only one slot (its type) once name is the stable
		// key, so "disjoint sub-fields" doesn't apply the same way: either both sides agree, or
		// they conflict, there's no third "different parts of the same field" case for a field
		// that's just (name, type).
		if inOurs == inTheirs && oursType == theirsType {
			continue // both sides made the identical change -- not a clash
		}
		switch {
		case !inOurs && !inTheirs:
			continue // both removed it -- identical change, already handled above, kept for clarity
		case !inOurs:
			clashes = append(clashes, fmt.Sprintf("field %q (removed here, retyped to %q on the other side)", name, theirsType))
		case !inTheirs:
			clashes = append(clashes, fmt.Sprintf("field %q (retyped here to %q, removed on the other side)", name, oursType))
		default:
			clashes = append(clashes, fmt.Sprintf("field %q's type (%q vs %q)", name, oursType, theirsType))
		}
	}

	if len(clashes) > 0 {
		return SameNodeResult{
			Mergeable: false,
			Conflict: &allelev1.ConflictReport{
				Id:                  ours.Name + "#same-node",
				Kind:                allelev1.ConflictKind_CONFLICT_KIND_SAME_NODE,
				Symbol:              ours.Name,
				OtherWorktreeId:     theirsWorktreeID,
				OtherSymbol:         theirs.Name,
				Explanation:         fmt.Sprintf("both sides changed %s's %s", ours.Name, strings.Join(clashes, " and ")),
				WouldTextuallyMerge: false,
			},
		}, nil
	}

	return SameNodeResult{Mergeable: true}, nil
}
