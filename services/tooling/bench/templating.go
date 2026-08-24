// This file implements the "{{ steps.<name>.result.<field> }}" template resolver
// described in the doc's Primitives section and scoped for Phase 3: walk a step's
// `with:` (a *structpb.Struct, after loading) recursively, and wherever a string
// value contains one or more `{{ steps.NAME.result.PATH }}` occurrences, substitute
// the actual value from that step's already-completed StepResult.Result.
//
// Two substitution shapes are supported, matching the doc's own worked example:
//
//   - A field whose value is *exactly* one template expression (e.g.
//     `request.id: "{{ steps.create-course.result.id }}"`) is replaced with the
//     referenced value's *native* structpb type (string/number/bool/struct/list) —
//     important so a numeric or boolean field flows through as a number/bool, not a
//     stringified one, when it's later marshaled back to JSON for a grpc-call
//     request.
//   - A field whose value *contains* a template expression alongside other text
//     (e.g. `message: "Seeded a test course: {{ steps.create-course.result.id }}"`,
//     from the doc's garage:// example) gets the matched portion(s) replaced with the
//     referenced value's string representation, leaving the rest of the string
//     intact.
//
// A step referencing a step that hasn't completed yet (or doesn't exist, or
// completed without a result) is a run-time error for the *referencing* step, not a
// load-time one — Phase 2 already rejects a depends_on that names an undeclared
// step, but a template can still reference a valid step name that simply hasn't run
// yet if a workflow's depends_on doesn't actually cover every field it templates
// from (a authoring mistake, not a graph cycle). See scheduler.go for how that
// surfaces as a step failure rather than a panic.
package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// templateRe matches `{{ steps.<name>.result.<path> }}`, tolerating extra
// whitespace around the pieces the way the doc's examples are formatted. <name>
// allows the step-name charset the loader validates (letters, digits, - and _);
// <path> additionally allows '.' for a dotted path into a nested result field.
var templateRe = regexp.MustCompile(`\{\{\s*steps\.([\w-]+)\.result\.([\w.-]+)\s*\}\}`)

// resolveTemplates returns a copy of `with` with every `{{ steps.X.result.Y }}`
// occurrence substituted using `completed` (keyed by step name, containing only
// steps that have already finished). A nil `with` returns nil, nil.
func resolveTemplates(with *structpb.Struct, completed map[string]*workflowv1.StepResult) (*structpb.Struct, error) {
	if with == nil {
		return nil, nil
	}
	resolved, err := resolveStructValue(with, completed)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func resolveStructValue(in *structpb.Struct, completed map[string]*workflowv1.StepResult) (*structpb.Struct, error) {
	out := &structpb.Struct{Fields: make(map[string]*structpb.Value, len(in.GetFields()))}
	for k, v := range in.GetFields() {
		rv, err := resolveValue(v, completed)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", k, err)
		}
		out.Fields[k] = rv
	}
	return out, nil
}

func resolveValue(v *structpb.Value, completed map[string]*workflowv1.StepResult) (*structpb.Value, error) {
	switch kind := v.GetKind().(type) {
	case *structpb.Value_StringValue:
		return resolveStringValue(kind.StringValue, completed)
	case *structpb.Value_StructValue:
		resolved, err := resolveStructValue(kind.StructValue, completed)
		if err != nil {
			return nil, err
		}
		return structpb.NewStructValue(resolved), nil
	case *structpb.Value_ListValue:
		items := make([]*structpb.Value, len(kind.ListValue.GetValues()))
		for i, item := range kind.ListValue.GetValues() {
			rv, err := resolveValue(item, completed)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			items[i] = rv
		}
		return structpb.NewListValue(&structpb.ListValue{Values: items}), nil
	default:
		return v, nil
	}
}

func resolveStringValue(s string, completed map[string]*workflowv1.StepResult) (*structpb.Value, error) {
	matches := templateRe.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return structpb.NewStringValue(s), nil
	}

	// The whole field is exactly one template expression: preserve the referenced
	// value's native type instead of stringifying it.
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) {
		stepName := s[matches[0][2]:matches[0][3]]
		path := s[matches[0][4]:matches[0][5]]
		return lookupStepResult(stepName, path, completed)
	}

	// One or more template expressions embedded in a larger string: substitute
	// each match's string representation in place.
	var sb strings.Builder
	last := 0
	for _, m := range matches {
		sb.WriteString(s[last:m[0]])
		stepName := s[m[2]:m[3]]
		path := s[m[4]:m[5]]
		val, err := lookupStepResult(stepName, path, completed)
		if err != nil {
			return nil, err
		}
		sb.WriteString(stringifyValue(val))
		last = m[1]
	}
	sb.WriteString(s[last:])
	return structpb.NewStringValue(sb.String()), nil
}

// lookupStepResult resolves `steps.<stepName>.result.<path>` against `completed`,
// returning a clear, specific error (never panicking) if the step hasn't run, has no
// recorded result, or the path doesn't exist within it.
func lookupStepResult(stepName, path string, completed map[string]*workflowv1.StepResult) (*structpb.Value, error) {
	sr, ok := completed[stepName]
	if !ok {
		return nil, fmt.Errorf("template references step %q, which has not completed (check depends_on)", stepName)
	}
	if sr.GetResult() == nil {
		return nil, fmt.Errorf("template references step %q, which has no recorded result (status: %s)", stepName, sr.GetStatus())
	}
	val, ok := lookupStructPath(sr.GetResult(), path)
	if !ok {
		return nil, fmt.Errorf("template references step %q result field %q, which does not exist", stepName, path)
	}
	return val, nil
}

// lookupStructPath walks a dotted path (e.g. "name.first_name") into a
// *structpb.Struct, returning the leaf value if every segment resolves.
func lookupStructPath(s *structpb.Struct, path string) (*structpb.Value, bool) {
	cur := structpb.NewStructValue(s)
	for _, part := range strings.Split(path, ".") {
		fields := cur.GetStructValue()
		if fields == nil {
			return nil, false
		}
		v, ok := fields.GetFields()[part]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// stringifyValue renders a structpb.Value for embedding into a larger string, e.g.
// the doc's `message: "Seeded a test course: {{ steps.create-course.result.id }}"`.
func stringifyValue(v *structpb.Value) string {
	switch v.GetKind().(type) {
	case *structpb.Value_StringValue:
		return v.GetStringValue()
	case *structpb.Value_NumberValue:
		return strconv.FormatFloat(v.GetNumberValue(), 'f', -1, 64)
	case *structpb.Value_BoolValue:
		return strconv.FormatBool(v.GetBoolValue())
	case *structpb.Value_NullValue, nil:
		return ""
	default:
		// struct/list values embedded in a larger string: fall back to their JSON
		// representation rather than dropping the data.
		b, err := protojson.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
