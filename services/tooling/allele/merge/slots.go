package merge

import (
	"fmt"

	"github.com/steady-bytes/draft/services/tooling/allele/parsing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// NamedTypedSlot is one (name, type) pair -- a function parameter or a struct field. Both Go
// grammar shapes decompose identically: a list node whose named children each carry "name"/"type"
// fields -- confirmed via a real probe (two parameters, two struct fields) before writing this.
//
// Scope, stated explicitly: this file only decomposes Go. Rust/proto would need their own
// grammar-specific slot extraction (Rust's `parameters`/field_declaration_list have different
// field names than Go's, and proto's grammar labels no fields at all -- see diff/symbols.go's own
// per-language split) -- real work, not a few-line variation, and the PRD's own worked examples
// for R2.2 are both Go. Deferred rather than guessed at here; Decisions covers this.
type NamedTypedSlot struct {
	Name string
	Type string
}

// FunctionSlots re-parses a function/method Symbol's own already-extracted Text in isolation
// (diff.Symbol.Text for a "function"/"method" kind -- the full declaration including the `func`
// keyword and, for a method, its receiver) and returns its parameters, positionally ("the Nth
// parameter" -- a rename is exactly a change *to* the name, so name can't be the matching key for
// parameters the way it is for top-level symbols), plus its result/return-type text. Confirmed
// empirically that a method_declaration's own full text re-parses cleanly standalone, with no
// prefix needed, before writing this.
func FunctionSlots(text string) (params []NamedTypedSlot, result string, err error) {
	tree, err := parsing.Parse(parsing.LanguageGo, []byte(text))
	if err != nil {
		return nil, "", err
	}
	defer tree.Close()

	root := tree.RootNode()
	if root.NamedChildCount() == 0 {
		return nil, "", fmt.Errorf("re-parsing function text produced an empty tree")
	}
	decl := root.NamedChild(0)
	if decl.Kind() != "function_declaration" && decl.Kind() != "method_declaration" {
		return nil, "", fmt.Errorf("re-parsing function text produced %q, not a function/method declaration", decl.Kind())
	}

	src := []byte(text)
	if paramList := decl.ChildByFieldName("parameters"); paramList != nil {
		params = namedTypedSlots(src, paramList)
	}
	if r := decl.ChildByFieldName("result"); r != nil {
		result = r.Utf8Text(src)
	}
	return params, result, nil
}

// StructFields re-parses a "type" Symbol's own Text (diff.Symbol.Text for a type_spec -- the spec
// alone, without a leading "type" keyword; see diff/symbols.go's extractGoSymbols) and returns its
// struct fields by name, or ok=false if the type isn't a struct (an interface, alias, etc. -- not
// a case this phase's worked examples need). A bare type_spec's text doesn't parse at the top
// level on its own -- confirmed empirically, it becomes an ERROR node -- so the "type" keyword is
// added back here purely for re-parsing; diff/symbols.go's own Text is correct as-is for Phase 5's
// text-equality comparisons, which never re-parse anything.
func StructFields(text string) (fields []NamedTypedSlot, ok bool, err error) {
	src := []byte("type " + text)
	tree, err := parsing.Parse(parsing.LanguageGo, src)
	if err != nil {
		return nil, false, err
	}
	defer tree.Close()

	root := tree.RootNode()
	if root.NamedChildCount() == 0 || root.NamedChild(0).Kind() != "type_declaration" {
		return nil, false, fmt.Errorf("re-parsing type text did not produce a type_declaration")
	}
	typeDecl := root.NamedChild(0)
	if typeDecl.NamedChildCount() == 0 {
		return nil, false, fmt.Errorf("re-parsed type_declaration has no type_spec")
	}
	spec := typeDecl.NamedChild(0)
	structType := spec.ChildByFieldName("type")
	if structType == nil || structType.Kind() != "struct_type" {
		return nil, false, nil // interface, alias, etc. -- not a struct, not an error
	}
	if structType.NamedChildCount() == 0 {
		return nil, true, nil // struct{} -- no fields
	}
	fieldList := structType.NamedChild(0) // field_declaration_list -- confirmed positional, no field label
	return namedTypedSlots(src, fieldList), true, nil
}

func namedTypedSlots(src []byte, list *tree_sitter.Node) []NamedTypedSlot {
	var out []NamedTypedSlot
	for i := uint(0); i < list.NamedChildCount(); i++ {
		item := list.NamedChild(i)
		name := item.ChildByFieldName("name")
		typ := item.ChildByFieldName("type")
		if name == nil || typ == nil {
			continue
		}
		out = append(out, NamedTypedSlot{Name: name.Utf8Text(src), Type: typ.Utf8Text(src)})
	}
	return out
}
