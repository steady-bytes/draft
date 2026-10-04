// Package linkage parses and diffs draft.linkage.yaml, the per-repository structural manifest
// describing a system's own components and their links (R5 -- see the plan's "The Linkage Manifest"
// section).
//
// Deliberately NOT tree-sitter-yaml, despite the plan's own original "parsed with tree-sitter-yaml"
// prose -- a real, disclosed deviation made once the actual tradeoff was visible, the same way
// Phase 10 deviated from its own plan's "trigger inputs" assumption once Bench's real API was
// inspected. The plan's own stated goal for using a real parser here -- "a key reorder or whitespace
// change never shows up as a change" -- holds just as well comparing fully-parsed, already-structured
// Go values (order-independent by construction, see Diff below) as it would comparing tree-sitter
// nodes. draft.linkage.yaml has one small, fixed, known schema -- the exact shape
// services/tooling/bench/loader.go already parses the identical way (plain yaml.v3 into a typed Go
// struct) for its own fixed-schema workflow YAML -- not the open-ended "any valid Go/Rust/.proto
// program" problem tree-sitter earns its keep solving elsewhere in this plan (parsing/, diff/,
// merge/). Adding a new grammar dependency for no additional correctness here would be complexity
// without benefit.
package linkage

import (
	"fmt"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"

	"gopkg.in/yaml.v3"
)

// ManifestPath is the file every repository commits to describe its own component/link graph.
const ManifestPath = "draft.linkage.yaml"

type yamlManifest struct {
	Components []yamlComponent `yaml:"components"`
}

type yamlComponent struct {
	Name    string     `yaml:"name"`
	Version string     `yaml:"version"`
	Links   []yamlLink `yaml:"links"`
}

type yamlLink struct {
	Target    string `yaml:"target"`
	Mode      string `yaml:"mode"`
	Transport string `yaml:"transport"`
}

// Parse decodes raw draft.linkage.yaml content into the typed Component read-model -- the same
// "derive at read time, never a parallel store" discipline this plan's own prose asks for, and
// Phase 7's computeChange already established for Change.
func Parse(src []byte) ([]*allelev1.Component, error) {
	var doc yamlManifest
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ManifestPath, err)
	}
	components := make([]*allelev1.Component, 0, len(doc.Components))
	for _, c := range doc.Components {
		links := make([]*allelev1.Link, 0, len(c.Links))
		for _, l := range c.Links {
			mode, err := parseLinkMode(l.Mode)
			if err != nil {
				return nil, fmt.Errorf("%s: component %q: %w", ManifestPath, c.Name, err)
			}
			links = append(links, &allelev1.Link{Target: l.Target, Mode: mode, Transport: l.Transport})
		}
		components = append(components, &allelev1.Component{Name: c.Name, Version: c.Version, Links: links})
	}
	return components, nil
}

func parseLinkMode(s string) (allelev1.LinkMode, error) {
	switch s {
	case "static":
		return allelev1.LinkMode_LINK_MODE_STATIC, nil
	case "runtime_service":
		return allelev1.LinkMode_LINK_MODE_RUNTIME_SERVICE, nil
	case "sidecar":
		return allelev1.LinkMode_LINK_MODE_SIDECAR, nil
	default:
		return allelev1.LinkMode_LINK_MODE_UNSPECIFIED, fmt.Errorf("unrecognized link mode %q (want static, runtime_service, or sidecar)", s)
	}
}

// Diff compares base/compare's already-parsed component lists -- not raw text or tree nodes -- for
// exactly the edges allele-linkage.html's own diff view needs: a link whose mode changed, or a
// component/link that exists on only one side. Component on the resulting LinkageManifestChange is
// the link's own target name, matching the proto's own documented example ("beacon-exporter: static
// -> sidecar") -- not a "source -> target" composite; a component's own identity (which source has
// the link) is implicit in which row of the UI's version table a change is attached to, the same way
// the mockup itself shows it.
func Diff(base, compare []*allelev1.Component) []*allelev1.LinkageManifestChange {
	baseComponents := indexByName(base)
	compareComponents := indexByName(compare)

	var changes []*allelev1.LinkageManifestChange

	// A component removed entirely: every one of its own links is reported removed.
	for name, bc := range baseComponents {
		if _, ok := compareComponents[name]; !ok {
			for _, l := range bc.GetLinks() {
				changes = append(changes, &allelev1.LinkageManifestChange{
					Component: l.GetTarget(), ModeBefore: l.GetMode(), Removed: true,
				})
			}
		}
	}
	// A component added entirely: every one of its own links is reported added.
	for name, cc := range compareComponents {
		if _, ok := baseComponents[name]; !ok {
			for _, l := range cc.GetLinks() {
				changes = append(changes, &allelev1.LinkageManifestChange{
					Component: l.GetTarget(), ModeAfter: l.GetMode(), Added: true,
				})
			}
		}
	}
	// A component present on both sides: diff its own links by target.
	for name, bc := range baseComponents {
		cc, ok := compareComponents[name]
		if !ok {
			continue
		}
		bLinks := indexLinksByTarget(bc)
		cLinks := indexLinksByTarget(cc)
		for target, bl := range bLinks {
			cl, ok := cLinks[target]
			switch {
			case !ok:
				changes = append(changes, &allelev1.LinkageManifestChange{Component: target, ModeBefore: bl.GetMode(), Removed: true})
			case bl.GetMode() != cl.GetMode():
				changes = append(changes, &allelev1.LinkageManifestChange{Component: target, ModeBefore: bl.GetMode(), ModeAfter: cl.GetMode()})
			}
		}
		for target, cl := range cLinks {
			if _, ok := bLinks[target]; !ok {
				changes = append(changes, &allelev1.LinkageManifestChange{Component: target, ModeAfter: cl.GetMode(), Added: true})
			}
		}
	}
	return changes
}

func indexByName(components []*allelev1.Component) map[string]*allelev1.Component {
	m := make(map[string]*allelev1.Component, len(components))
	for _, c := range components {
		m[c.GetName()] = c
	}
	return m
}

func indexLinksByTarget(c *allelev1.Component) map[string]*allelev1.Link {
	m := make(map[string]*allelev1.Link, len(c.GetLinks()))
	for _, l := range c.GetLinks() {
		m[l.GetTarget()] = l
	}
	return m
}

// IsBreaking reports whether changes should be treated as a breaking set of edits under policy --
// R5.4's own "always / interface-only / never" three-way choice.
//
// ALWAYS and INTERFACE_ONLY are honestly identical here, not an oversight: LinkageManifestChange
// itself (component, mode_before, mode_after, added, removed) has no field at all for a non-interface
// difference -- a component's own version bump with no link change isn't something Diff emits a
// change for in the first place (there's no version_before/version_after on the message to report
// it through). Every change this phase's Diff can possibly produce already *is* a link mode/add/
// remove -- the interface, by this plan's own "Breaking only if the interface changes" framing (the
// mockup's own caption) -- so there is currently no non-interface case for ALWAYS to be stricter
// about. The policy is still stored and threaded through correctly (SetBreakingChangePolicy,
// GetLinkageManifest); the two policies only start behaving differently once a future phase adds a
// representable non-interface change (e.g. version-only bumps) for ALWAYS to actually catch that
// INTERFACE_ONLY wouldn't.
func IsBreaking(changes []*allelev1.LinkageManifestChange, policy allelev1.BreakingChangePolicy) bool {
	if policy == allelev1.BreakingChangePolicy_BREAKING_CHANGE_POLICY_NEVER {
		return false
	}
	return len(changes) > 0
}
