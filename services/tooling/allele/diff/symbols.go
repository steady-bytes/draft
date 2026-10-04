package diff

import (
	"strings"

	"github.com/steady-bytes/draft/services/tooling/allele/parsing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Symbol is one named declaration extracted from a parsed file -- the unit SymbolChange reports
// on, matching exactly what allele-change.html's own symbol tree shows as a row: a function, a
// method (qualified by its receiver/impl type), a top-level type, or (for .proto) a message/enum.
//
// Field names, not positional child indexing, drive extraction for Go and Rust -- confirmed via a
// real probe file parsed with cmd/parse before writing this (see the plan's Phase 5 completion
// note). Proto's own grammar (coder3101/tree-sitter-proto) labels no fields at all, confirmed the
// same way; its extraction below is positional instead, not an oversight.
//
// Text is extracted eagerly, here, while the source tree is still alive -- Symbol deliberately
// holds no *tree_sitter.Node. A first version of this package did keep the Node and called
// Utf8Text on it later from diff.go's compareSymbol, after symbolsFor's own `defer tree.Close()`
// had already run -- a real, live-reproduced SIGSEGV (cgo use-after-free: Close frees the
// underlying C tree, and the stored Node became a dangling pointer into it), not a theoretical
// concern. Capturing the text up front, while the tree backing it is still open, removes the
// footgun entirely rather than asking every future caller to remember a tree-lifetime rule.
type Symbol struct {
	Name      string // "Pick", "Weighted.Pick" for a method, "probe.Weighted" is NOT qualified by package -- see Decisions below
	Kind      string // "function" | "method" | "type" | "struct" | "enum" | "message" -- mirrors SymbolChange.kind, display only
	Signature string // parameters+result/return_type text, excluding the body; "" for kinds with no meaningful signature (type/struct/enum/message)
	Text      string // the whole symbol's own source text (name+signature+body) -- what compareSymbol diffs
}

// ExtractSymbols walks a parsed tree's top-level declarations (and, for Go/Rust, methods nested
// one level inside a type/impl block) into a flat Symbol list. Unrecognized top-level node kinds
// (imports, package clauses, comments, top-level var/const in Rust, ...) are silently skipped --
// they're not symbols a change-detail page would ever show a row for.
func ExtractSymbols(lang parsing.Language, src []byte, root *tree_sitter.Node) []Symbol {
	switch lang {
	case parsing.LanguageGo:
		return extractGoSymbols(src, root)
	case parsing.LanguageRust:
		return extractRustSymbols(src, root)
	case parsing.LanguageProto:
		return extractProtoSymbols(src, root)
	default:
		return nil
	}
}

// -- Go --------------------------------------------------------------------------------------

func extractGoSymbols(src []byte, root *tree_sitter.Node) []Symbol {
	var out []Symbol
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		switch child.Kind() {
		case "function_declaration":
			name := child.ChildByFieldName("name")
			if name == nil {
				continue
			}
			out = append(out, Symbol{
				Name:      name.Utf8Text(src),
				Kind:      "function",
				Signature: goFuncSignature(src, child),
				Text:      child.Utf8Text(src),
			})
		case "method_declaration":
			name := child.ChildByFieldName("name")
			recv := child.ChildByFieldName("receiver")
			if name == nil || recv == nil {
				continue
			}
			recvType := goReceiverTypeName(src, recv)
			if recvType == "" {
				continue
			}
			out = append(out, Symbol{
				Name:      recvType + "." + name.Utf8Text(src),
				Kind:      "method",
				Signature: goFuncSignature(src, child),
				Text:      child.Utf8Text(src),
			})
		case "type_declaration":
			// Holds one or more type_spec children (`type X struct{}` is the single-spec case;
			// `type ( A struct{}; B int )` is the rarer multi-spec one) -- one Symbol per spec.
			for j := uint(0); j < child.NamedChildCount(); j++ {
				spec := child.NamedChild(j)
				if spec.Kind() != "type_spec" {
					continue
				}
				name := spec.ChildByFieldName("name")
				if name == nil {
					continue
				}
				out = append(out, Symbol{
					Name: name.Utf8Text(src),
					Kind: "type",
					Text: spec.Utf8Text(src), // whole spec (incl. its struct/interface body), not just the name
				})
			}
		}
	}
	return out
}

// goReceiverTypeName extracts "Weighted" from a method's receiver parameter_list, handling both
// pointer ((w *Weighted)) and value ((w Weighted)) receivers -- confirmed against a real pointer
// receiver via cmd/parse; value receivers skip the extra pointer_type unwrap, same field access.
func goReceiverTypeName(src []byte, receiver *tree_sitter.Node) string {
	if receiver.NamedChildCount() == 0 {
		return ""
	}
	param := receiver.NamedChild(0) // parameter_declaration
	t := param.ChildByFieldName("type")
	if t == nil {
		return ""
	}
	if t.Kind() == "pointer_type" && t.NamedChildCount() > 0 {
		t = t.NamedChild(0)
	}
	return t.Utf8Text(src)
}

// goFuncSignature returns the parameters+result text, excluding the body -- what
// SymbolChange.signature_before/signature_after compare, matching allele-change.html's own
// "func (w *Weighted) Pick(u *route.Upstream) Endpoint" -> "...(u *route.Upstream, h
// *HealthWindow) (Endpoint, error)" display.
func goFuncSignature(src []byte, fn *tree_sitter.Node) string {
	var b strings.Builder
	if params := fn.ChildByFieldName("parameters"); params != nil {
		b.WriteString(params.Utf8Text(src))
	}
	if result := fn.ChildByFieldName("result"); result != nil {
		b.WriteByte(' ')
		b.WriteString(result.Utf8Text(src))
	}
	return strings.TrimSpace(b.String())
}

// -- Rust ------------------------------------------------------------------------------------

func extractRustSymbols(src []byte, root *tree_sitter.Node) []Symbol {
	var out []Symbol
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		switch child.Kind() {
		case "function_item":
			if s, ok := rustFunctionSymbol(src, child, ""); ok {
				out = append(out, s)
			}
		case "struct_item":
			if name := child.ChildByFieldName("name"); name != nil {
				out = append(out, Symbol{Name: name.Utf8Text(src), Kind: "struct", Text: child.Utf8Text(src)})
			}
		case "enum_item":
			if name := child.ChildByFieldName("name"); name != nil {
				out = append(out, Symbol{Name: name.Utf8Text(src), Kind: "enum", Text: child.Utf8Text(src)})
			}
		case "impl_item":
			implType := child.ChildByFieldName("type")
			if implType == nil {
				continue
			}
			typeName := implType.Utf8Text(src)
			body := child.ChildByFieldName("body")
			if body == nil {
				continue
			}
			for j := uint(0); j < body.NamedChildCount(); j++ {
				m := body.NamedChild(j)
				if m.Kind() != "function_item" {
					continue
				}
				if s, ok := rustFunctionSymbol(src, m, typeName); ok {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// rustFunctionSymbol builds a Symbol for a function_item. prefix is the impl's own type name for
// a method ("Weighted"), or "" for a free-standing function -- Kind and Name both depend on which.
func rustFunctionSymbol(src []byte, fn *tree_sitter.Node, prefix string) (Symbol, bool) {
	name := fn.ChildByFieldName("name")
	if name == nil {
		return Symbol{}, false
	}
	n := name.Utf8Text(src)
	kind := "function"
	if prefix != "" {
		n = prefix + "." + n
		kind = "method"
	}
	return Symbol{Name: n, Kind: kind, Signature: rustFuncSignature(src, fn), Text: fn.Utf8Text(src)}, true
}

func rustFuncSignature(src []byte, fn *tree_sitter.Node) string {
	var b strings.Builder
	if params := fn.ChildByFieldName("parameters"); params != nil {
		b.WriteString(params.Utf8Text(src))
	}
	if ret := fn.ChildByFieldName("return_type"); ret != nil {
		b.WriteString(" -> ")
		b.WriteString(ret.Utf8Text(src))
	}
	return strings.TrimSpace(b.String())
}

// -- Proto -----------------------------------------------------------------------------------

// extractProtoSymbols uses positional child access, not ChildByFieldName -- confirmed via a real
// probe file that coder3101/tree-sitter-proto labels no fields at all (every other language here
// does), so NamedChild(0) is the correct, deliberate way to reach a message/enum's name node, not
// a fallback for a missing field label.
func extractProtoSymbols(src []byte, root *tree_sitter.Node) []Symbol {
	var out []Symbol
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		switch child.Kind() {
		case "message":
			if child.NamedChildCount() == 0 {
				continue
			}
			nameNode := child.NamedChild(0) // message_name
			out = append(out, Symbol{Name: nameNode.Utf8Text(src), Kind: "message", Text: child.Utf8Text(src)})
		case "enum":
			if child.NamedChildCount() == 0 {
				continue
			}
			nameNode := child.NamedChild(0) // enum_name
			out = append(out, Symbol{Name: nameNode.Utf8Text(src), Kind: "enum", Text: child.Utf8Text(src)})
		}
	}
	return out
}
