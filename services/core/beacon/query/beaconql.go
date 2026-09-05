// Package query implements BeaconQL — the small WHERE-clause-style filter
// grammar described in the design doc's Query Language section: comparisons,
// AND/OR, LIKE, IN, and map-index attribute access (`attributes["route"]`).
//
// A parsed expression is used two ways:
//   - Compile turns it into a parameterized ClickHouse SQL fragment (`?`
//     placeholders + a matching []any of bound values) for QueryLogs and the
//     historical-replay half of StreamLogs. Column names come exclusively from
//     a fixed switch in columnFor — never from raw user text — so user input
//     only ever reaches ClickHouse as a bound parameter value, never
//     interpolated into the query string itself.
//   - Matches evaluates the same AST directly against an in-memory store.LogRow
//     for StreamLogs' live-tail path, so newly-ingested rows can be filtered
//     without a round trip to ClickHouse per row.
package query

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/steady-bytes/draft/services/core/beacon/store"
)

// ParseError is returned for any malformed BeaconQL *or* PromQL-subset
// expression (see promql.go, which reuses this exact type rather than
// defining its own) so callers (rpc.go's toConnectError) can map it to
// connect.CodeInvalidArgument with a clear message, rather than letting a
// malformed filter/query reach ClickHouse and surface as an opaque database
// error. Grammar-agnostic on purpose — Msg alone (each parser's own wording)
// carries the "which grammar, what's wrong" context, so this doesn't
// hardcode a "beaconql:"-style prefix that would be misleading coming from
// promql.go.
type ParseError struct {
	Msg string
}

func (e *ParseError) Error() string { return e.Msg }

// ─── AST ────────────────────────────────────────────────────────────────────

type Expr any

type BinaryExpr struct {
	Left  Expr
	Op    string // "AND" | "OR"
	Right Expr
}

type FieldRef struct {
	Name   string
	MapKey *string // set for attributes["key"] / resource_attributes["key"]
}

type Literal struct {
	Str *string
	Num *float64
}

type ComparisonExpr struct {
	Field FieldRef
	Op    string // "=", "!=", "<", "<=", ">", ">="
	Value Literal
}

type LikeExpr struct {
	Field   FieldRef
	Pattern string
	Negated bool
}

type InExpr struct {
	Field   FieldRef
	Values  []Literal
	Negated bool
}

// ─── Lexer ──────────────────────────────────────────────────────────────────

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokAnd
	tokOr
	tokNot
	tokLike
	tokIn
	tokEq
	tokNeq
	tokLt
	tokLte
	tokGt
	tokGte
	tokComma
)

type token struct {
	kind tokenKind
	text string
}

type lexer struct {
	input []rune
	pos   int
}

func newLexer(s string) *lexer {
	return &lexer{input: []rune(s)}
}

func (l *lexer) peekRune() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\n') {
		l.pos++
	}
	if l.pos >= len(l.input) {
		return token{kind: tokEOF}, nil
	}

	r := l.input[l.pos]
	switch r {
	case '(':
		l.pos++
		return token{kind: tokLParen}, nil
	case ')':
		l.pos++
		return token{kind: tokRParen}, nil
	case '[':
		l.pos++
		return token{kind: tokLBracket}, nil
	case ']':
		l.pos++
		return token{kind: tokRBracket}, nil
	case ',':
		l.pos++
		return token{kind: tokComma}, nil
	case '=':
		l.pos++
		return token{kind: tokEq}, nil
	case '!':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return token{kind: tokNeq}, nil
		}
		return token{}, &ParseError{Msg: "unexpected '!' (did you mean '!=' ?)"}
	case '<':
		l.pos++
		if l.peekRune() == '=' {
			l.pos++
			return token{kind: tokLte}, nil
		}
		return token{kind: tokLt}, nil
	case '>':
		l.pos++
		if l.peekRune() == '=' {
			l.pos++
			return token{kind: tokGte}, nil
		}
		return token{kind: tokGt}, nil
	case '"', '\'':
		return l.lexString(r)
	}

	if r == '-' || r == '.' || (r >= '0' && r <= '9') {
		return l.lexNumber()
	}
	if isIdentStart(r) {
		return l.lexIdent()
	}
	return token{}, &ParseError{Msg: fmt.Sprintf("unexpected character %q", r)}
}

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9') || r == '.'
}

func (l *lexer) lexString(quote rune) (token, error) {
	start := l.pos
	l.pos++ // consume opening quote
	var sb strings.Builder
	for {
		if l.pos >= len(l.input) {
			return token{}, &ParseError{Msg: fmt.Sprintf("unterminated string starting at position %d", start)}
		}
		r := l.input[l.pos]
		if r == '\\' && l.pos+1 < len(l.input) {
			sb.WriteRune(l.input[l.pos+1])
			l.pos += 2
			continue
		}
		if r == quote {
			l.pos++
			break
		}
		sb.WriteRune(r)
		l.pos++
	}
	return token{kind: tokString, text: sb.String()}, nil
}

func (l *lexer) lexNumber() (token, error) {
	start := l.pos
	l.pos++
	for l.pos < len(l.input) && ((l.input[l.pos] >= '0' && l.input[l.pos] <= '9') || l.input[l.pos] == '.') {
		l.pos++
	}
	text := string(l.input[start:l.pos])
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		return token{}, &ParseError{Msg: fmt.Sprintf("invalid number %q", text)}
	}
	return token{kind: tokNumber, text: text}, nil
}

func (l *lexer) lexIdent() (token, error) {
	start := l.pos
	for l.pos < len(l.input) && isIdentPart(l.input[l.pos]) {
		l.pos++
	}
	text := string(l.input[start:l.pos])
	switch strings.ToUpper(text) {
	case "AND":
		return token{kind: tokAnd}, nil
	case "OR":
		return token{kind: tokOr}, nil
	case "NOT":
		return token{kind: tokNot}, nil
	case "LIKE":
		return token{kind: tokLike}, nil
	case "IN":
		return token{kind: tokIn}, nil
	default:
		return token{kind: tokIdent, text: text}, nil
	}
}

// ─── Parser ─────────────────────────────────────────────────────────────────

type parser struct {
	lex *lexer
	cur token
	err error
}

func newParser(s string) (*parser, error) {
	p := &parser{lex: newLexer(s)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *parser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = t
	return nil
}

// ParseBeaconQL parses a BeaconQL expression into an AST. An empty (or
// whitespace-only) filter string is valid and means "no filter" — it returns a
// nil Expr, not an error.
func ParseBeaconQL(filter string) (Expr, error) {
	if strings.TrimSpace(filter) == "" {
		return nil, nil
	}
	p, err := newParser(filter)
	if err != nil {
		return nil, err
	}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != tokEOF {
		return nil, &ParseError{Msg: fmt.Sprintf("unexpected trailing input near %q", p.cur.text)}
	}
	return expr, nil
}

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokOr {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: "OR", Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokAnd {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: "AND", Right: right}
	}
	return left, nil
}

func (p *parser) parsePrimary() (Expr, error) {
	if p.cur.kind == tokLParen {
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur.kind != tokRParen {
			return nil, &ParseError{Msg: "expected closing ')'"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return inner, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (Expr, error) {
	field, err := p.parseFieldRef()
	if err != nil {
		return nil, err
	}

	negated := false
	if p.cur.kind == tokNot {
		negated = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	switch p.cur.kind {
	case tokLike:
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.kind != tokString {
			return nil, &ParseError{Msg: "expected string pattern after LIKE"}
		}
		pattern := p.cur.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &LikeExpr{Field: field, Pattern: pattern, Negated: negated}, nil

	case tokIn:
		if err := p.advance(); err != nil {
			return nil, err
		}
		values, err := p.parseValueList()
		if err != nil {
			return nil, err
		}
		return &InExpr{Field: field, Values: values, Negated: negated}, nil
	}

	if negated {
		return nil, &ParseError{Msg: "NOT is only valid before LIKE or IN"}
	}

	op, err := p.parseCompOp()
	if err != nil {
		return nil, err
	}
	value, err := p.parseLiteral()
	if err != nil {
		return nil, err
	}
	return &ComparisonExpr{Field: field, Op: op, Value: value}, nil
}

func (p *parser) parseFieldRef() (FieldRef, error) {
	if p.cur.kind != tokIdent {
		return FieldRef{}, &ParseError{Msg: "expected a field name"}
	}
	name := p.cur.text
	if err := p.advance(); err != nil {
		return FieldRef{}, err
	}
	if p.cur.kind != tokLBracket {
		return FieldRef{Name: name}, nil
	}
	if err := p.advance(); err != nil {
		return FieldRef{}, err
	}
	if p.cur.kind != tokString {
		return FieldRef{}, &ParseError{Msg: fmt.Sprintf("expected a quoted map key after %s[", name)}
	}
	key := p.cur.text
	if err := p.advance(); err != nil {
		return FieldRef{}, err
	}
	if p.cur.kind != tokRBracket {
		return FieldRef{}, &ParseError{Msg: "expected closing ']'"}
	}
	if err := p.advance(); err != nil {
		return FieldRef{}, err
	}
	return FieldRef{Name: name, MapKey: &key}, nil
}

func (p *parser) parseCompOp() (string, error) {
	var op string
	switch p.cur.kind {
	case tokEq:
		op = "="
	case tokNeq:
		op = "!="
	case tokLt:
		op = "<"
	case tokLte:
		op = "<="
	case tokGt:
		op = ">"
	case tokGte:
		op = ">="
	default:
		return "", &ParseError{Msg: "expected a comparison operator (=, !=, <, <=, >, >=), LIKE, or IN"}
	}
	if err := p.advance(); err != nil {
		return "", err
	}
	return op, nil
}

func (p *parser) parseLiteral() (Literal, error) {
	switch p.cur.kind {
	case tokString:
		v := p.cur.text
		if err := p.advance(); err != nil {
			return Literal{}, err
		}
		return Literal{Str: &v}, nil
	case tokNumber:
		v, err := strconv.ParseFloat(p.cur.text, 64)
		if err != nil {
			return Literal{}, &ParseError{Msg: fmt.Sprintf("invalid number %q", p.cur.text)}
		}
		if err := p.advance(); err != nil {
			return Literal{}, err
		}
		return Literal{Num: &v}, nil
	default:
		return Literal{}, &ParseError{Msg: "expected a string or number literal"}
	}
}

func (p *parser) parseValueList() ([]Literal, error) {
	if p.cur.kind != tokLParen {
		return nil, &ParseError{Msg: "expected '(' after IN"}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	var values []Literal
	for {
		v, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		values = append(values, v)
		if p.cur.kind == tokComma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	if p.cur.kind != tokRParen {
		return nil, &ParseError{Msg: "expected ',' or ')' in value list"}
	}
	if err := p.advance(); err != nil { // consume ')'
		return nil, err
	}
	if len(values) == 0 {
		return nil, &ParseError{Msg: "IN (...) requires at least one value"}
	}
	return values, nil
}

// ─── Compile: AST → parameterized ClickHouse SQL ───────────────────────────

// Compile turns expr into a WHERE-clause fragment using `?` placeholders and a
// matching slice of bound arguments. It never interpolates user-supplied text
// into the returned SQL string — only fixed column names (chosen via a closed
// switch in columnFor) appear literally; every user-supplied value (including
// map keys) is returned as a bound argument.
func Compile(expr Expr) (string, []any, error) {
	switch e := expr.(type) {
	case nil:
		return "", nil, nil
	case *BinaryExpr:
		lsql, largs, err := Compile(e.Left)
		if err != nil {
			return "", nil, err
		}
		rsql, rargs, err := Compile(e.Right)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("(%s) %s (%s)", lsql, e.Op, rsql), append(largs, rargs...), nil
	case *ComparisonExpr:
		col, colArgs, err := columnFor(e.Field)
		if err != nil {
			return "", nil, err
		}
		args := append(colArgs, literalValue(e.Value))
		return col + " " + e.Op + " ?", args, nil
	case *LikeExpr:
		col, colArgs, err := columnFor(e.Field)
		if err != nil {
			return "", nil, err
		}
		kw := "LIKE"
		if e.Negated {
			kw = "NOT LIKE"
		}
		args := append(colArgs, e.Pattern)
		return col + " " + kw + " ?", args, nil
	case *InExpr:
		col, colArgs, err := columnFor(e.Field)
		if err != nil {
			return "", nil, err
		}
		placeholders := make([]string, len(e.Values))
		args := colArgs
		for i, v := range e.Values {
			placeholders[i] = "?"
			args = append(args, literalValue(v))
		}
		kw := "IN"
		if e.Negated {
			kw = "NOT IN"
		}
		return col + " " + kw + " (" + strings.Join(placeholders, ", ") + ")", args, nil
	default:
		return "", nil, &ParseError{Msg: "unrecognized expression node"}
	}
}

// columnFor maps a BeaconQL field name to a fixed ClickHouse column expression.
// This is the injection boundary: the returned string is always one of the
// literal cases below, never assembled from f.Name — only a map key (bound as
// a `?` parameter, in colArgs) ever flows from user input into the query.
func columnFor(f FieldRef) (col string, colArgs []any, err error) {
	name := strings.ToLower(f.Name)
	switch name {
	case "attributes":
		if f.MapKey == nil {
			return "", nil, &ParseError{Msg: `attributes requires a map key, e.g. attributes["route"]`}
		}
		return "attributes[?]", []any{*f.MapKey}, nil
	case "resource_attributes":
		if f.MapKey == nil {
			return "", nil, &ParseError{Msg: `resource_attributes requires a map key, e.g. resource_attributes["route"]`}
		}
		return "resource_attributes[?]", []any{*f.MapKey}, nil
	case "severity":
		if f.MapKey != nil {
			return "", nil, &ParseError{Msg: "field \"severity\" does not support map-index access"}
		}
		return "severity_text", nil, nil
	case "timestamp", "trace_id", "span_id", "severity_text", "severity_number", "service_name", "body":
		if f.MapKey != nil {
			return "", nil, &ParseError{Msg: fmt.Sprintf("field %q does not support map-index access", f.Name)}
		}
		return name, nil, nil
	default:
		return "", nil, &ParseError{Msg: fmt.Sprintf("unknown field %q", f.Name)}
	}
}

func literalValue(l Literal) any {
	switch {
	case l.Str != nil:
		return *l.Str
	case l.Num != nil:
		return *l.Num
	default:
		return nil
	}
}

// ─── Compile for trace_roots ────────────────────────────────────────────────
//
// SearchTraces (query/traces.go) reuses the same lexer/parser/AST as
// QueryLogs — the design doc's "extend BeaconQL to work against trace_roots's
// columns" instruction, not a second parser. Only the column mapping differs,
// since trace_roots has a different shape than logs. CompileTraceRoot mirrors
// Compile's structure exactly; the two are kept as separate functions (rather
// than threading a column-schema parameter through one shared Compile)
// because trace_roots additionally exposes a synthetic `duration_ms` field —
// friendlier than requiring callers to know duration is stored in
// nanoseconds — which needs a per-field numeric-literal scale that plain
// Compile has no concept of.
func CompileTraceRoot(expr Expr) (string, []any, error) {
	switch e := expr.(type) {
	case nil:
		return "", nil, nil
	case *BinaryExpr:
		lsql, largs, err := CompileTraceRoot(e.Left)
		if err != nil {
			return "", nil, err
		}
		rsql, rargs, err := CompileTraceRoot(e.Right)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("(%s) %s (%s)", lsql, e.Op, rsql), append(largs, rargs...), nil
	case *ComparisonExpr:
		col, colArgs, scale, err := columnForTraceRoot(e.Field)
		if err != nil {
			return "", nil, err
		}
		args := append(colArgs, scaledLiteralValue(e.Value, scale))
		return col + " " + e.Op + " ?", args, nil
	case *LikeExpr:
		col, colArgs, _, err := columnForTraceRoot(e.Field)
		if err != nil {
			return "", nil, err
		}
		kw := "LIKE"
		if e.Negated {
			kw = "NOT LIKE"
		}
		args := append(colArgs, e.Pattern)
		return col + " " + kw + " ?", args, nil
	case *InExpr:
		col, colArgs, scale, err := columnForTraceRoot(e.Field)
		if err != nil {
			return "", nil, err
		}
		placeholders := make([]string, len(e.Values))
		args := colArgs
		for i, v := range e.Values {
			placeholders[i] = "?"
			args = append(args, scaledLiteralValue(v, scale))
		}
		kw := "IN"
		if e.Negated {
			kw = "NOT IN"
		}
		return col + " " + kw + " (" + strings.Join(placeholders, ", ") + ")", args, nil
	default:
		return "", nil, &ParseError{Msg: "unrecognized expression node"}
	}
}

// columnForTraceRoot maps a BeaconQL field name to a fixed trace_roots column
// expression, mirroring columnFor's injection boundary: only the literal
// cases below are ever returned, never a string assembled from f.Name. The
// third return value scales a numeric comparison literal before it's bound —
// used only by duration_ms, which trace_roots stores as duration_ns.
func columnForTraceRoot(f FieldRef) (col string, colArgs []any, scale float64, err error) {
	name := strings.ToLower(f.Name)
	switch name {
	case "attributes":
		return "", nil, 0, &ParseError{Msg: `attributes is not queryable on trace_roots — use GetTrace to inspect a trace's individual spans`}
	case "trace_id", "service_name", "span_name", "status_code", "start_time":
		if f.MapKey != nil {
			return "", nil, 0, &ParseError{Msg: fmt.Sprintf("field %q does not support map-index access", f.Name)}
		}
		return name, nil, 1, nil
	case "duration_ns":
		if f.MapKey != nil {
			return "", nil, 0, &ParseError{Msg: `field "duration_ns" does not support map-index access`}
		}
		return "duration_ns", nil, 1, nil
	case "duration_ms":
		if f.MapKey != nil {
			return "", nil, 0, &ParseError{Msg: `field "duration_ms" does not support map-index access`}
		}
		return "duration_ns", nil, 1_000_000, nil
	default:
		return "", nil, 0, &ParseError{Msg: fmt.Sprintf("unknown field %q", f.Name)}
	}
}

// scaledLiteralValue applies columnForTraceRoot's numeric scale (e.g.
// duration_ms's 1e6 to reach duration_ns) to a comparison literal before it's
// bound as a query parameter. Non-numeric literals (strings) and a scale of 1
// pass through literalValue unchanged.
func scaledLiteralValue(l Literal, scale float64) any {
	if l.Num != nil && scale != 1 {
		v := *l.Num * scale
		return v
	}
	return literalValue(l)
}

// ─── Compile for wide_events ────────────────────────────────────────────────
//
// CompileWideEvent mirrors CompileTraceRoot's structure exactly, against
// wide_events' own columns instead. See
// docs/website/content/docs/architecture/wide-events.md's Data Model for the
// schema this maps onto.
func CompileWideEvent(expr Expr) (string, []any, error) {
	switch e := expr.(type) {
	case nil:
		return "", nil, nil
	case *BinaryExpr:
		lsql, largs, err := CompileWideEvent(e.Left)
		if err != nil {
			return "", nil, err
		}
		rsql, rargs, err := CompileWideEvent(e.Right)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("(%s) %s (%s)", lsql, e.Op, rsql), append(largs, rargs...), nil
	case *ComparisonExpr:
		col, colArgs, err := columnForWideEvent(e.Field)
		if err != nil {
			return "", nil, err
		}
		args := append(colArgs, literalValue(e.Value))
		return col + " " + e.Op + " ?", args, nil
	case *LikeExpr:
		col, colArgs, err := columnForWideEvent(e.Field)
		if err != nil {
			return "", nil, err
		}
		kw := "LIKE"
		if e.Negated {
			kw = "NOT LIKE"
		}
		args := append(colArgs, e.Pattern)
		return col + " " + kw + " ?", args, nil
	case *InExpr:
		col, colArgs, err := columnForWideEvent(e.Field)
		if err != nil {
			return "", nil, err
		}
		placeholders := make([]string, len(e.Values))
		args := colArgs
		for i, v := range e.Values {
			placeholders[i] = "?"
			args = append(args, literalValue(v))
		}
		kw := "IN"
		if e.Negated {
			kw = "NOT IN"
		}
		return col + " " + kw + " (" + strings.Join(placeholders, ", ") + ")", args, nil
	default:
		return "", nil, &ParseError{Msg: "unrecognized expression node"}
	}
}

// columnForWideEvent maps a BeaconQL field name to a fixed wide_events column
// expression, mirroring columnFor/columnForTraceRoot's injection boundary:
// only the literal cases below are ever returned, never a string assembled
// from f.Name.
func columnForWideEvent(f FieldRef) (col string, colArgs []any, err error) {
	name := strings.ToLower(f.Name)
	switch name {
	case "attributes":
		if f.MapKey == nil {
			return "", nil, &ParseError{Msg: `attributes requires a map key, e.g. attributes["http.method"]`}
		}
		return "attributes[?]", []any{*f.MapKey}, nil
	case "business_attributes":
		if f.MapKey == nil {
			return "", nil, &ParseError{Msg: `business_attributes requires a map key, e.g. business_attributes["user_id"]`}
		}
		return "business_attributes[?]", []any{*f.MapKey}, nil
	case "runtime_attributes":
		if f.MapKey == nil {
			return "", nil, &ParseError{Msg: `runtime_attributes requires a map key, e.g. runtime_attributes["host"]`}
		}
		return "runtime_attributes[?]", []any{*f.MapKey}, nil
	case "trace_id", "span_id", "parent_span_id", "service_name", "span_name", "start_time", "duration_ns", "status_code":
		if f.MapKey != nil {
			return "", nil, &ParseError{Msg: fmt.Sprintf("field %q does not support map-index access", f.Name)}
		}
		return name, nil, nil
	default:
		return "", nil, &ParseError{Msg: fmt.Sprintf("unknown field %q", f.Name)}
	}
}

// ─── Matches: AST evaluated in-memory against a store.LogRow ──────────────
//
// Used for StreamLogs' live-tail path so newly-ingested rows can be filtered
// without a ClickHouse round trip. Semantics intentionally mirror Compile.

func Matches(expr Expr, row store.LogRow) bool {
	switch e := expr.(type) {
	case nil:
		return true
	case *BinaryExpr:
		if e.Op == "AND" {
			return Matches(e.Left, row) && Matches(e.Right, row)
		}
		return Matches(e.Left, row) || Matches(e.Right, row)
	case *ComparisonExpr:
		val, ok := fieldValue(e.Field, row)
		if !ok {
			return false
		}
		return compareValues(val, e.Op, e.Value)
	case *LikeExpr:
		val, ok := fieldValue(e.Field, row)
		if !ok {
			return false
		}
		matched := likeMatch(fmt.Sprint(val), e.Pattern)
		if e.Negated {
			return !matched
		}
		return matched
	case *InExpr:
		val, ok := fieldValue(e.Field, row)
		if !ok {
			return false
		}
		found := false
		for _, v := range e.Values {
			if literalEquals(val, v) {
				found = true
				break
			}
		}
		if e.Negated {
			return !found
		}
		return found
	default:
		return false
	}
}

func fieldValue(f FieldRef, row store.LogRow) (any, bool) {
	name := strings.ToLower(f.Name)
	switch name {
	case "attributes":
		if f.MapKey == nil {
			return nil, false
		}
		v, ok := row.Attributes[*f.MapKey]
		return v, ok
	case "resource_attributes":
		if f.MapKey == nil {
			return nil, false
		}
		v, ok := row.ResourceAttributes[*f.MapKey]
		return v, ok
	case "timestamp":
		return row.Timestamp.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), true
	case "trace_id":
		return row.TraceID, true
	case "span_id":
		return row.SpanID, true
	case "severity", "severity_text":
		return row.SeverityText, true
	case "severity_number":
		return float64(row.SeverityNumber), true
	case "service_name":
		return row.ServiceName, true
	case "body":
		return row.Body, true
	default:
		return nil, false
	}
}

func compareValues(val any, op string, lit Literal) bool {
	if lit.Num != nil {
		fv, ok := toFloat(val)
		if !ok {
			return false
		}
		return numericCompare(fv, op, *lit.Num)
	}
	if lit.Str != nil {
		sv := fmt.Sprint(val)
		return stringCompare(sv, op, *lit.Str)
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func numericCompare(a float64, op string, b float64) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	default:
		return false
	}
}

func stringCompare(a string, op string, b string) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	default:
		return false
	}
}

func literalEquals(val any, lit Literal) bool {
	if lit.Num != nil {
		fv, ok := toFloat(val)
		return ok && fv == *lit.Num
	}
	if lit.Str != nil {
		return fmt.Sprint(val) == *lit.Str
	}
	return false
}

var likeRegexCache = map[string]*regexp.Regexp{}

func likeMatch(s, pattern string) bool {
	re, ok := likeRegexCache[pattern]
	if !ok {
		var sb strings.Builder
		sb.WriteString("^")
		for _, r := range pattern {
			switch r {
			case '%':
				sb.WriteString(".*")
			case '_':
				sb.WriteString(".")
			default:
				sb.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
		sb.WriteString("$")
		compiled, err := regexp.Compile(sb.String())
		if err != nil {
			return false
		}
		re = compiled
		likeRegexCache[pattern] = re
	}
	return re.MatchString(s)
}
