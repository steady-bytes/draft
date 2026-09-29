// This file backs the workflow editor's live feedback: given the YAML being typed, say where it is
// wrong (line and column), and draw the trigger → step pipeline. It is advisory. Saving still goes
// through ParseWorkflow (loader.go), the one authority on what a valid workflow is; analyzeWorkflow
// adds positions that ParseWorkflow's single first-error message cannot give, plus checks a saved
// file never needed (unknown keys, `with:` against a plugin's published config_schema).
package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// problem is one thing wrong with a workflow document.
type problem struct {
	// Line and Col are 1-based; Line 0 means the problem is not tied to a position.
	Line, Col int
	// Warn problems do not stop a save; the rest do.
	Warn    bool
	Message string
}

// Pos is "24:7", or "—" for a problem with no position.
func (p problem) Pos() string {
	if p.Line == 0 {
		return "—"
	}
	if p.Col == 0 {
		return fmt.Sprintf("%d", p.Line)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

// pipeNode is one box of the pipeline strip.
type pipeNode struct {
	Trigger bool
	// Label is the small caps line ("Webhook", "01"); Name is the text after it.
	Label, Name string
	Err         bool
}

// analysis is what the editor shows under and above the text area.
type analysis struct {
	Lines    int
	Problems []problem
	Pipe     []pipeNode
	// Checked names the plugins whose config_schema the `with:` blocks were validated against.
	Checked []string
	// SchemaUnavailable is set when a registry could not be reached, so `with:` was not checked.
	SchemaUnavailable bool
}

// Errors counts the problems that would block a save.
func (a analysis) Errors() int {
	n := 0
	for _, p := range a.Problems {
		if !p.Warn {
			n++
		}
	}
	return n
}

// ErrorLines are the lines to mark in the gutter.
func (a analysis) ErrorLines() []int {
	var lines []int
	seen := map[int]bool{}
	for _, p := range a.Problems {
		if p.Line > 0 && !p.Warn && !seen[p.Line] {
			seen[p.Line] = true
			lines = append(lines, p.Line)
		}
	}
	sort.Ints(lines)
	return lines
}

// pluginSchema is the part of a plugin's config_schema the editor checks.
type pluginSchema struct {
	Required []string
	// Properties are the declared keys; empty means the schema does not restrict them.
	Properties map[string]bool
}

// schemaLookup returns the schema of foundry://name@version: (nil, nil) when no registry publishes
// it, and an error when a registry could not be asked.
type schemaLookup func(ctx context.Context, name, version string) (*pluginSchema, error)

var (
	topLevelKeys = []string{"apiVersion", "kind", "metadata", "trigger", "steps"}
	stepKeys     = []string{"name", "uses", "with", "expect", "depends_on", "on_failure", "retry"}
	yamlLineRe   = regexp.MustCompile(`line (\d+)`)
)

// analyzeWorkflow reads src. lookup may be nil, which skips the plugin schema checks.
func analyzeWorkflow(ctx context.Context, src string, lookup schemaLookup) analysis {
	a := analysis{Lines: strings.Count(src, "\n") + 1}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		a.Problems = append(a.Problems, yamlSyntaxProblems(err)...)
		return a
	}
	if root.Kind == 0 || len(root.Content) == 0 {
		a.Problems = append(a.Problems, problem{Line: 1, Col: 1, Message: "The document is empty."})
		return a
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		a.Problems = append(a.Problems, problem{Line: doc.Line, Col: doc.Column, Message: "A workflow is a mapping with metadata, trigger and steps."})
		return a
	}

	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i]
		if !contains(topLevelKeys, key.Value) {
			a.Problems = append(a.Problems, unknownKey(key, "a workflow", topLevelKeys, true))
		}
	}

	// A wrong type anywhere (steps written as a mapping, say) is reported at the offending line.
	var parsed yamlWorkflowDoc
	if err := root.Decode(&parsed); err != nil {
		a.Problems = append(a.Problems, yamlSyntaxProblems(err)...)
	}

	a.checkMetadata(doc)
	stepRanges := a.checkSteps(ctx, doc, &parsed, lookup)
	a.buildPipeline(&parsed, stepRanges)

	// Whatever ParseWorkflow rejects must show up here, so a Save never fails for a reason the
	// editor did not already name.
	if a.Errors() == 0 {
		if _, err := ParseWorkflow([]byte(src)); err != nil {
			a.Problems = append(a.Problems, problem{Message: strings.TrimPrefix(err.Error(), "failed to parse workflow yaml: ")})
		}
	}

	sort.SliceStable(a.Problems, func(i, j int) bool {
		pi, pj := a.Problems[i], a.Problems[j]
		if (pi.Line == 0) != (pj.Line == 0) {
			return pi.Line != 0
		}
		if pi.Line != pj.Line {
			return pi.Line < pj.Line
		}
		return pi.Col < pj.Col
	})
	return a
}

func (a *analysis) checkMetadata(doc *yaml.Node) {
	metaKey, meta := mapEntry(doc, "metadata")
	if meta == nil || meta.Kind != yaml.MappingNode {
		line, col := 1, 1
		if metaKey != nil {
			line, col = metaKey.Line, metaKey.Column
		}
		a.Problems = append(a.Problems, problem{Line: line, Col: col, Message: "metadata.name is required."})
		return
	}
	if k, v := mapEntry(meta, "name"); k == nil || strings.TrimSpace(v.Value) == "" {
		a.Problems = append(a.Problems, problem{Line: metaKey.Line, Col: metaKey.Column, Message: "metadata.name is required."})
	}
	for i := 0; i+1 < len(meta.Content); i += 2 {
		if k := meta.Content[i]; k.Value != "name" && k.Value != "description" {
			a.Problems = append(a.Problems, unknownKey(k, "metadata", []string{"name", "description"}, true))
		}
	}
}

// stepRange is the lines a step occupies, for marking its pipeline node.
type stepRange struct{ start, end int }

func (a *analysis) checkSteps(ctx context.Context, doc *yaml.Node, parsed *yamlWorkflowDoc, lookup schemaLookup) []stepRange {
	stepsKey, steps := mapEntry(doc, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode || len(steps.Content) == 0 {
		line, col := 1, 1
		if stepsKey != nil {
			line, col = stepsKey.Line, stepsKey.Column
		}
		a.Problems = append(a.Problems, problem{Line: line, Col: col, Message: "At least one step is required."})
		return nil
	}

	ranges := make([]stepRange, len(steps.Content))
	for i, s := range steps.Content {
		end := a.Lines
		if i+1 < len(steps.Content) {
			end = steps.Content[i+1].Line - 1
		}
		ranges[i] = stepRange{start: s.Line, end: end}
	}

	names := map[string]bool{}
	for _, s := range steps.Content {
		if _, v := mapEntry(s, "name"); v != nil && strings.TrimSpace(v.Value) != "" {
			names[v.Value] = true
		}
	}

	seen := map[string]bool{}
	schemas := map[string]*pluginSchema{}
	unreachable := map[string]bool{}
	checked := map[string]bool{}
	for _, s := range steps.Content {
		if s.Kind != yaml.MappingNode {
			a.Problems = append(a.Problems, problem{Line: s.Line, Col: s.Column, Message: "A step is a mapping with name and uses."})
			continue
		}
		for i := 0; i+1 < len(s.Content); i += 2 {
			if k := s.Content[i]; !contains(stepKeys, k.Value) {
				a.Problems = append(a.Problems, unknownKey(k, "a step", stepKeys, true))
			}
		}

		_, name := mapEntry(s, "name")
		switch {
		case name == nil || strings.TrimSpace(name.Value) == "":
			a.Problems = append(a.Problems, problem{Line: s.Line, Col: s.Column, Message: "This step needs a name."})
		case seen[name.Value]:
			a.Problems = append(a.Problems, problem{Line: name.Line, Col: name.Column, Message: fmt.Sprintf("Another step is already called %q.", name.Value)})
		default:
			seen[name.Value] = true
		}

		usesKey, uses := mapEntry(s, "uses")
		switch {
		case uses == nil || strings.TrimSpace(uses.Value) == "":
			line, col := s.Line, s.Column
			if usesKey != nil {
				line, col = usesKey.Line, usesKey.Column
			}
			a.Problems = append(a.Problems, problem{Line: line, Col: col, Message: "uses is required."})
		default:
			scheme, _, hasScheme := strings.Cut(uses.Value, "://")
			if !hasScheme || (scheme != "bench" && scheme != "foundry") {
				a.Problems = append(a.Problems, problem{Line: uses.Line, Col: uses.Column, Message: fmt.Sprintf("uses %q must start with bench:// or foundry://.", uses.Value)})
			} else if scheme == "foundry" {
				a.checkPluginConfig(ctx, s, uses, lookup, schemas, unreachable, checked)
			}
		}

		if _, deps := mapEntry(s, "depends_on"); deps != nil && deps.Kind == yaml.SequenceNode {
			for _, d := range deps.Content {
				if !names[d.Value] {
					a.Problems = append(a.Problems, problem{Line: d.Line, Col: d.Column, Message: fmt.Sprintf("depends_on names %q, which is not a step here.", d.Value)})
				}
			}
		}
		if _, of := mapEntry(s, "on_failure"); of != nil {
			if v := strings.ToLower(of.Value); v != "fail" && v != "continue" && v != "retry" {
				a.Problems = append(a.Problems, problem{Line: of.Line, Col: of.Column, Message: fmt.Sprintf("on_failure %q is not one of fail, continue, retry.", of.Value)})
			}
		}
	}

	if cycle := cycleStep(parsed); cycle != "" {
		for _, s := range steps.Content {
			if _, n := mapEntry(s, "name"); n != nil && n.Value == cycle {
				a.Problems = append(a.Problems, problem{Line: n.Line, Col: n.Column, Message: fmt.Sprintf("Step %q is part of a depends_on cycle.", cycle)})
				break
			}
		}
	}

	for n := range checked {
		a.Checked = append(a.Checked, n)
	}
	sort.Strings(a.Checked)
	a.SchemaUnavailable = len(unreachable) > 0
	return ranges
}

// checkPluginConfig validates a foundry:// step's `with:` against the plugin's published schema.
func (a *analysis) checkPluginConfig(ctx context.Context, step, uses *yaml.Node, lookup schemaLookup, schemas map[string]*pluginSchema, unreachable, checked map[string]bool) {
	name, version, err := parseFoundryUses(uses.Value)
	if err != nil {
		a.Problems = append(a.Problems, problem{Line: uses.Line, Col: uses.Column, Message: "Expected foundry://<name>@<version>."})
		return
	}
	if lookup == nil {
		return
	}
	ref := name + "@" + version
	schema, known := schemas[ref]
	if !known && !unreachable[ref] {
		schema, err = lookup(ctx, name, version)
		if err != nil {
			unreachable[ref] = true
			return
		}
		schemas[ref] = schema
	}
	if unreachable[ref] {
		return
	}
	if schema == nil {
		a.Problems = append(a.Problems, problem{Line: uses.Line, Col: uses.Column, Warn: true, Message: fmt.Sprintf("%s is not published to any configured registry.", ref)})
		return
	}
	checked[ref] = true

	withKey, with := mapEntry(step, "with")
	given := map[string]bool{}
	if with != nil && with.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(with.Content); i += 2 {
			k := with.Content[i]
			given[k.Value] = true
			if len(schema.Properties) > 0 && !schema.Properties[k.Value] {
				msg := fmt.Sprintf("%s is not a field of %s.", k.Value, ref)
				if hint := closest(k.Value, keysOf(schema.Properties)); hint != "" {
					msg += fmt.Sprintf(" Did you mean %s?", hint)
				}
				a.Problems = append(a.Problems, problem{Line: k.Line, Col: k.Column, Message: msg})
			}
		}
	}
	line, col := step.Line, step.Column
	if withKey != nil {
		line, col = withKey.Line, withKey.Column
	} else {
		line, col = uses.Line, uses.Column
	}
	for _, req := range schema.Required {
		if !given[req] {
			a.Problems = append(a.Problems, problem{Line: line, Col: col, Message: fmt.Sprintf("%s is required by %s.", req, ref)})
		}
	}
}

func (a *analysis) buildPipeline(parsed *yamlWorkflowDoc, ranges []stepRange) {
	if wh := parsed.Trigger.Webhook; wh != nil {
		name := "webhook"
		if wh.Slug != "" {
			name = "/webhooks/" + wh.Slug
		}
		a.Pipe = append(a.Pipe, pipeNode{Trigger: true, Label: "Webhook", Name: name})
	} else {
		a.Pipe = append(a.Pipe, pipeNode{Trigger: true, Label: "Manual", Name: "run now"})
	}
	for i, s := range parsed.Steps {
		n := pipeNode{Label: fmt.Sprintf("%02d", i+1), Name: s.Name}
		if n.Name == "" {
			n.Name = "unnamed"
		}
		if i < len(ranges) {
			for _, p := range a.Problems {
				if !p.Warn && p.Line >= ranges[i].start && p.Line <= ranges[i].end {
					n.Err = true
					break
				}
			}
		}
		a.Pipe = append(a.Pipe, n)
	}
}

// cycleStep names a step on a depends_on cycle, or "" when the graph is acyclic.
func cycleStep(doc *yamlWorkflowDoc) string {
	steps := make(map[string][]string, len(doc.Steps))
	order := make([]string, 0, len(doc.Steps))
	for i, s := range doc.Steps {
		deps := s.DependsOn
		if len(deps) == 0 && i > 0 {
			deps = []string{doc.Steps[i-1].Name} // the implicit "previous step" edge
		}
		steps[s.Name] = deps
		order = append(order, s.Name)
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var found string
	var visit func(n string) bool
	visit = func(n string) bool {
		switch state[n] {
		case visiting:
			found = n
			return true
		case done:
			return false
		}
		state[n] = visiting
		for _, d := range steps[n] {
			if _, ok := steps[d]; ok && visit(d) {
				return true
			}
		}
		state[n] = done
		return false
	}
	for _, n := range order {
		if visit(n) {
			return found
		}
	}
	return ""
}

// yamlSyntaxProblems turns yaml.v3's errors ("yaml: line 3: did not find expected key") into
// positioned problems.
func yamlSyntaxProblems(err error) []problem {
	var msgs []string
	if te, ok := err.(*yaml.TypeError); ok {
		msgs = te.Errors
	} else {
		msgs = []string{err.Error()}
	}
	out := make([]problem, 0, len(msgs))
	for _, m := range msgs {
		p := problem{Col: 1, Message: strings.TrimSpace(strings.TrimPrefix(m, "yaml: "))}
		if match := yamlLineRe.FindStringSubmatch(m); match != nil {
			fmt.Sscanf(match[1], "%d", &p.Line)
			p.Message = strings.TrimSpace(strings.Replace(p.Message, "line "+match[1]+": ", "", 1))
		}
		if p.Line == 0 {
			p.Col = 0
		}
		out = append(out, p)
	}
	return out
}

func unknownKey(k *yaml.Node, where string, known []string, warn bool) problem {
	msg := fmt.Sprintf("%s is not a field of %s.", k.Value, where)
	if hint := closest(k.Value, known); hint != "" {
		msg += fmt.Sprintf(" Did you mean %s?", hint)
	}
	return problem{Line: k.Line, Col: k.Column, Warn: warn, Message: msg}
}

// mapEntry finds key in a mapping node, returning its key and value nodes (nil, nil if absent).
func mapEntry(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// closest is the candidate within two edits of word (the best one), or "".
func closest(word string, candidates []string) string {
	best, bestD := "", 3
	for _, c := range candidates {
		if d := editDistance(strings.ToLower(word), strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// workflowMeta reads metadata.name and description out of the text, tolerating an invalid document
// (the editor fields are filled from whatever can be read).
func workflowMeta(src string) (name, description string) {
	var doc yamlWorkflowDoc
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return "", ""
	}
	return doc.Metadata.Name, doc.Metadata.Description
}
