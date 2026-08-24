// Package query (this file) implements the scoped PromQL subset described in
// the design doc's Query Language section: instant/range vector selectors
// with label matchers (`metric_name{label="value"}`), `rate()`, `sum/avg/
// max/min by(...)`, and simple scalar arithmetic — compiled to bounded,
// parameterized ClickHouse queries over `metric_points`.
//
// This is its own lexer/parser, not a reuse of BeaconQL's grammar (the two
// languages have different syntax — braces/brackets/functions vs. a
// WHERE-clause) — but it mirrors BeaconQL's error-handling convention
// exactly: any malformed or unsupported construct returns the same
// *ParseError type defined in beaconql.go, so rpc.go's toConnectError maps
// both grammars' failures to CodeInvalidArgument with a clear message,
// never a raw ClickHouse error or a silent misinterpretation.
//
// Evaluation strategy: like Prometheus's own query engine, this does not try
// to push rate()/aggregation windowing into SQL. CompileSelector turns a
// vector selector's label matchers into a parameterized WHERE fragment (the
// injection boundary — see its doc comment), store.Storer.QueryMetricPoints
// runs that against ClickHouse to fetch bounded raw (timestamp, value)
// points, and Evaluate does the actual rate/aggregation/arithmetic math over
// those points in Go — the same split Prometheus itself has between its TSDB
// (raw sample storage) and its query engine (function evaluation).
package query

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/steady-bytes/draft/services/core/beacon/store"
)

// ─── AST ────────────────────────────────────────────────────────────────────

// PromExpr is any node in a parsed PromQL-subset expression:
// *VectorSelector, *CallExpr, *AggExpr, *NumberLiteral, or *PromBinaryExpr.
type PromExpr any

// LabelMatcher is one `name = "value"` / `name != "value"` constraint inside
// a vector selector's `{...}`.
type LabelMatcher struct {
	Name  string
	Op    string // "=" | "!="
	Value string
}

// VectorSelector is `metric_name{matchers...}` optionally followed by a
// `[duration]` range — the range is only meaningful (and only accepted) as
// rate()'s argument; a bare selector used anywhere else is treated as an
// instant/resampled vector.
type VectorSelector struct {
	MetricName string
	Matchers   []LabelMatcher
	RangeDur   *time.Duration
}

// CallExpr is a function call. Only "rate" is supported in this subset.
type CallExpr struct {
	Func string
	Arg  *VectorSelector
}

// AggExpr is `sum|avg|max|min [by (labels...)] (expr)` (either `by`
// placement PromQL itself allows — before or after the parenthesized
// argument). Expr is restricted to *VectorSelector or *CallExpr — nested
// aggregations/arithmetic as an aggregation's argument are an explicitly
// unsupported construct in this deliberately small subset (see
// parseAggInnerExpr).
type AggExpr struct {
	Op   string // "sum" | "avg" | "max" | "min"
	By   []string
	Expr PromExpr
}

// NumberLiteral is a bare scalar, valid only as one operand of a
// *PromBinaryExpr in this subset (see PromBinaryExpr's doc comment).
type NumberLiteral struct {
	Value float64
}

// PromBinaryExpr is `left op right` for op in {+, -, *, /}. This subset only
// supports *scalar* arithmetic (one operand must be a *NumberLiteral) —
// vector-to-vector binary operations (label-set matching/`on`/`ignoring`)
// are an explicitly unsupported construct; Evaluate returns a clear
// *ParseError for them rather than guessing a join semantic.
type PromBinaryExpr struct {
	Left  PromExpr
	Op    string // "+" | "-" | "*" | "/"
	Right PromExpr
}

// ─── Lexer ──────────────────────────────────────────────────────────────────

type promTokenKind int

const (
	promTokEOF promTokenKind = iota
	promTokIdent
	promTokString
	promTokNumber // also used for duration literals like "5m" — the parser
	// decides which parse (strconv.ParseFloat vs. time.ParseDuration) applies
	// based on where the token appears.
	promTokLBrace
	promTokRBrace
	promTokLBracket
	promTokRBracket
	promTokLParen
	promTokRParen
	promTokComma
	promTokEq
	promTokNeq
	promTokPlus
	promTokMinus
	promTokStar
	promTokSlash
)

type promToken struct {
	kind promTokenKind
	text string
}

type promLexer struct {
	input []rune
	pos   int
}

func newPromLexer(s string) *promLexer {
	return &promLexer{input: []rune(s)}
}

func (l *promLexer) next() (promToken, error) {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\n') {
		l.pos++
	}
	if l.pos >= len(l.input) {
		return promToken{kind: promTokEOF}, nil
	}

	r := l.input[l.pos]
	switch r {
	case '{':
		l.pos++
		return promToken{kind: promTokLBrace}, nil
	case '}':
		l.pos++
		return promToken{kind: promTokRBrace}, nil
	case '[':
		l.pos++
		return promToken{kind: promTokLBracket}, nil
	case ']':
		l.pos++
		return promToken{kind: promTokRBracket}, nil
	case '(':
		l.pos++
		return promToken{kind: promTokLParen}, nil
	case ')':
		l.pos++
		return promToken{kind: promTokRParen}, nil
	case ',':
		l.pos++
		return promToken{kind: promTokComma}, nil
	case '+':
		l.pos++
		return promToken{kind: promTokPlus}, nil
	case '-':
		l.pos++
		return promToken{kind: promTokMinus}, nil
	case '*':
		l.pos++
		return promToken{kind: promTokStar}, nil
	case '/':
		l.pos++
		return promToken{kind: promTokSlash}, nil
	case '=':
		l.pos++
		return promToken{kind: promTokEq}, nil
	case '!':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return promToken{kind: promTokNeq}, nil
		}
		return promToken{}, &ParseError{Msg: "unexpected '!' (did you mean '!=' ?)"}
	case '"', '\'':
		return l.lexString(r)
	}

	if (r >= '0' && r <= '9') || r == '.' {
		return l.lexNumberOrDuration()
	}
	if isIdentStart(r) {
		return l.lexIdent()
	}
	return promToken{}, &ParseError{Msg: fmt.Sprintf("unexpected character %q", r)}
}

func (l *promLexer) lexString(quote rune) (promToken, error) {
	start := l.pos
	l.pos++ // consume opening quote
	var sb strings.Builder
	for {
		if l.pos >= len(l.input) {
			return promToken{}, &ParseError{Msg: fmt.Sprintf("unterminated string starting at position %d", start)}
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
	return promToken{kind: promTokString, text: sb.String()}, nil
}

// lexNumberOrDuration consumes a numeric prefix (digits/'.') followed
// optionally by trailing letters (a duration unit, e.g. "5m", "30s", "1h").
// Which interpretation is valid is a parser-level (not lexer-level) concern —
// see parsePrimary (plain number) vs. parseVectorSelectorFrom (range
// duration).
func (l *promLexer) lexNumberOrDuration() (promToken, error) {
	start := l.pos
	for l.pos < len(l.input) && ((l.input[l.pos] >= '0' && l.input[l.pos] <= '9') || l.input[l.pos] == '.') {
		l.pos++
	}
	for l.pos < len(l.input) && isIdentStart(l.input[l.pos]) {
		l.pos++
	}
	return promToken{kind: promTokNumber, text: string(l.input[start:l.pos])}, nil
}

func (l *promLexer) lexIdent() (promToken, error) {
	start := l.pos
	for l.pos < len(l.input) && (isIdentStart(l.input[l.pos]) || (l.input[l.pos] >= '0' && l.input[l.pos] <= '9')) {
		l.pos++
	}
	return promToken{kind: promTokIdent, text: string(l.input[start:l.pos])}, nil
}

// ─── Parser ─────────────────────────────────────────────────────────────────

type promParser struct {
	lex *promLexer
	cur promToken
}

func newPromParser(s string) (*promParser, error) {
	p := &promParser{lex: newPromLexer(s)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *promParser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = t
	return nil
}

// ParsePromQL parses a PromQL-subset query string into an AST. An empty (or
// whitespace-only) query is always an error — unlike BeaconQL's empty
// filter, QueryMetrics has no "no query" meaning.
func ParsePromQL(q string) (PromExpr, error) {
	if strings.TrimSpace(q) == "" {
		return nil, &ParseError{Msg: "query is empty"}
	}
	p, err := newPromParser(q)
	if err != nil {
		return nil, err
	}
	expr, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != promTokEOF {
		return nil, &ParseError{Msg: fmt.Sprintf("unexpected trailing input near %q", p.cur.text)}
	}
	return expr, nil
}

func (p *promParser) parseAdditive() (PromExpr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == promTokPlus || p.cur.kind == promTokMinus {
		op := "+"
		if p.cur.kind == promTokMinus {
			op = "-"
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &PromBinaryExpr{Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *promParser) parseMultiplicative() (PromExpr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == promTokStar || p.cur.kind == promTokSlash {
		op := "*"
		if p.cur.kind == promTokSlash {
			op = "/"
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &PromBinaryExpr{Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *promParser) parsePrimary() (PromExpr, error) {
	switch p.cur.kind {
	case promTokNumber:
		v, err := strconv.ParseFloat(p.cur.text, 64)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("expected a number, got %q", p.cur.text)}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &NumberLiteral{Value: v}, nil
	case promTokLParen:
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		if p.cur.kind != promTokRParen {
			return nil, &ParseError{Msg: "expected closing ')'"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return inner, nil
	case promTokIdent:
		return p.parseIdentExpr()
	default:
		return nil, &ParseError{Msg: fmt.Sprintf("unexpected token %q", p.cur.text)}
	}
}

// parseIdentExpr dispatches an identifier to an aggregation (sum/avg/max/
// min), the rate() function, or a bare vector selector — the three shapes an
// identifier can start in this subset. Any other function/aggregation name
// followed by '(' is an explicitly unsupported construct.
func (p *promParser) parseIdentExpr() (PromExpr, error) {
	name := p.cur.text
	if err := p.advance(); err != nil {
		return nil, err
	}

	switch strings.ToLower(name) {
	case "sum", "avg", "max", "min":
		return p.parseAggExpr(strings.ToLower(name))
	case "rate":
		if p.cur.kind != promTokLParen {
			return nil, &ParseError{Msg: "expected '(' after rate"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		sel, err := p.parseVectorSelector()
		if err != nil {
			return nil, err
		}
		if sel.RangeDur == nil {
			return nil, &ParseError{Msg: "rate() requires a range-vector argument, e.g. rate(metric_name[5m])"}
		}
		if p.cur.kind != promTokRParen {
			return nil, &ParseError{Msg: "expected closing ')' after rate(...)"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &CallExpr{Func: "rate", Arg: sel}, nil
	default:
		if p.cur.kind == promTokLParen {
			return nil, &ParseError{Msg: fmt.Sprintf("unsupported function or aggregation %q — this PromQL subset only supports rate() and sum/avg/max/min by(...)", name)}
		}
		return p.parseVectorSelectorFrom(name)
	}
}

// parseAggExpr accepts both orderings PromQL itself allows:
// `sum by (labels) (expr)` and `sum (expr) by (labels)` — as well as no
// `by` clause at all (aggregate to a single series).
func (p *promParser) parseAggExpr(op string) (PromExpr, error) {
	var by []string

	if p.cur.kind == promTokIdent && strings.EqualFold(p.cur.text, "by") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		labels, err := p.parseLabelNameList()
		if err != nil {
			return nil, err
		}
		by = labels
	}

	if p.cur.kind != promTokLParen {
		return nil, &ParseError{Msg: fmt.Sprintf("expected '(' after %s", op)}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	inner, err := p.parseAggInnerExpr()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != promTokRParen {
		return nil, &ParseError{Msg: "expected closing ')'"}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}

	if by == nil && p.cur.kind == promTokIdent && strings.EqualFold(p.cur.text, "by") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		labels, err := p.parseLabelNameList()
		if err != nil {
			return nil, err
		}
		by = labels
	}

	return &AggExpr{Op: op, By: by, Expr: inner}, nil
}

// parseAggInnerExpr restricts an aggregation's argument to a vector selector
// or a rate(...) call — nested aggregations/arithmetic as an aggregation
// argument are an explicitly unsupported construct in this deliberately
// small subset, so callers get a clear error instead of a query that quietly
// only evaluates part of the expression.
func (p *promParser) parseAggInnerExpr() (PromExpr, error) {
	inner, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	switch inner.(type) {
	case *VectorSelector, *CallExpr:
		return inner, nil
	default:
		return nil, &ParseError{Msg: "aggregation argument must be a vector selector (e.g. metric_name{...}) or rate(...) — nested aggregations/arithmetic are not supported in this PromQL subset"}
	}
}

func (p *promParser) parseLabelNameList() ([]string, error) {
	if p.cur.kind != promTokLParen {
		return nil, &ParseError{Msg: "expected '(' after by"}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	var names []string
	for {
		if p.cur.kind != promTokIdent {
			return nil, &ParseError{Msg: "expected a label name in by(...)"}
		}
		names = append(names, p.cur.text)
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.kind == promTokComma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	if p.cur.kind != promTokRParen {
		return nil, &ParseError{Msg: "expected ',' or ')' in by(...)"}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return names, nil
}

// parseVectorSelector parses a metric name followed by the shared
// {matchers}[range] suffix — used by rate(), which requires a bare
// identifier immediately after its '('.
func (p *promParser) parseVectorSelector() (*VectorSelector, error) {
	if p.cur.kind != promTokIdent {
		return nil, &ParseError{Msg: "expected a metric name"}
	}
	name := p.cur.text
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p.parseVectorSelectorFrom(name)
}

func (p *promParser) parseVectorSelectorFrom(name string) (*VectorSelector, error) {
	sel := &VectorSelector{MetricName: name}

	if p.cur.kind == promTokLBrace {
		if err := p.advance(); err != nil {
			return nil, err
		}
		matchers, err := p.parseLabelMatchers()
		if err != nil {
			return nil, err
		}
		sel.Matchers = matchers
		if p.cur.kind != promTokRBrace {
			return nil, &ParseError{Msg: "expected closing '}'"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	if p.cur.kind == promTokLBracket {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.kind != promTokNumber {
			return nil, &ParseError{Msg: "expected a duration inside [...], e.g. [5m]"}
		}
		dur, err := time.ParseDuration(p.cur.text)
		if err != nil {
			return nil, &ParseError{Msg: fmt.Sprintf("invalid duration %q: %v", p.cur.text, err)}
		}
		if dur <= 0 {
			return nil, &ParseError{Msg: "range duration must be positive"}
		}
		sel.RangeDur = &dur
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.kind != promTokRBracket {
			return nil, &ParseError{Msg: "expected closing ']'"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	return sel, nil
}

func (p *promParser) parseLabelMatchers() ([]LabelMatcher, error) {
	var matchers []LabelMatcher
	if p.cur.kind == promTokRBrace {
		return matchers, nil
	}
	for {
		if p.cur.kind != promTokIdent {
			return nil, &ParseError{Msg: "expected a label name"}
		}
		name := p.cur.text
		if err := p.advance(); err != nil {
			return nil, err
		}

		var op string
		switch p.cur.kind {
		case promTokEq:
			op = "="
		case promTokNeq:
			op = "!="
		default:
			return nil, &ParseError{Msg: "expected '=' or '!=' in label matcher"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}

		if p.cur.kind != promTokString {
			return nil, &ParseError{Msg: "expected a quoted string value in label matcher"}
		}
		value := p.cur.text
		if err := p.advance(); err != nil {
			return nil, err
		}

		matchers = append(matchers, LabelMatcher{Name: name, Op: op, Value: value})
		if p.cur.kind == promTokComma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	return matchers, nil
}

// ─── CompileSelector: label matchers → parameterized ClickHouse fragment ───

// CompileSelector turns matchers into a `labels[?] = ?` / `labels[?] != ?`
// WHERE-clause fragment ANDed together, using `?` placeholders. This is the
// injection boundary: a label's *name* is bound as a query parameter (the
// map-index key) exactly like BeaconQL's attributes["key"] pattern
// (columnFor in beaconql.go) — never interpolated into the SQL string — so
// there is no fixed-column allowlist to maintain here at all, unlike
// BeaconQL's columnFor.
func CompileSelector(matchers []LabelMatcher) (string, []any, error) {
	if len(matchers) == 0 {
		return "", nil, nil
	}
	conds := make([]string, 0, len(matchers))
	args := make([]any, 0, len(matchers)*2)
	for _, m := range matchers {
		switch m.Op {
		case "=":
			conds = append(conds, "labels[?] = ?")
		case "!=":
			conds = append(conds, "labels[?] != ?")
		default:
			return "", nil, &ParseError{Msg: fmt.Sprintf("unsupported label matcher operator %q", m.Op)}
		}
		args = append(args, m.Name, m.Value)
	}
	return strings.Join(conds, " AND "), args, nil
}

// ─── Evaluate: AST → time series over a step grid ──────────────────────────

// MetricSample is one (timestamp, value) point of a MetricSeries.
type MetricSample struct {
	Timestamp time.Time
	Value     float64
}

// MetricSeries is one distinct label set's worth of samples produced by
// Evaluate — mirrors the QueryMetricsResponse.TimeSeries proto shape
// (rpc.go's seriesToProto converts directly).
type MetricSeries struct {
	MetricName string
	Labels     map[string]string
	Samples    []MetricSample
}

// maxPromQLSteps bounds the sample grid a single QueryMetrics evaluation
// walks — mirrors store.go's defaultLimit/maxLimit convention, protecting
// against a client requesting an unbounded number of ClickHouse round trips
// (bare selectors/rate() each fetch once per distinct metric+range, but the
// grid itself is walked in Go per series).
const maxPromQLSteps = 1_440

// point is one raw (timestamp, value) sample fetched from ClickHouse.
type point struct {
	ts  time.Time
	val float64
}

// rawSeries is every fetched point for one distinct label set of one metric.
type rawSeries struct {
	labels map[string]string
	points []point
}

// stepSeries is one distinct label set's values resampled onto a uniform
// step grid — has[i] is false where no value could be computed for step i
// (no data in that step's window), so an incomplete series doesn't silently
// read as a zero.
type stepSeries struct {
	metricName string
	labels     map[string]string
	values     []float64
	has        []bool
}

// Evaluate walks expr, fetching raw points from s as needed and computing
// rate/aggregation/arithmetic over a uniform [start, end] grid stepped by
// step, and returns one MetricSeries per distinct label set the expression
// produces (only steps with an actual computed value are included in each
// series' Samples).
func Evaluate(ctx context.Context, s store.Storer, expr PromExpr, start, end time.Time, step time.Duration) ([]MetricSeries, error) {
	if !start.Before(end) {
		return nil, &ParseError{Msg: "start must be strictly before end"}
	}
	if step <= 0 {
		return nil, &ParseError{Msg: "step must be positive"}
	}

	grid := stepGrid(start, end, step)
	if len(grid) > maxPromQLSteps {
		return nil, &ParseError{Msg: fmt.Sprintf("query spans too many steps (%d > %d) — narrow the time range or increase step", len(grid), maxPromQLSteps)}
	}

	result, err := evalExpr(ctx, s, expr, grid, step)
	if err != nil {
		return nil, err
	}

	out := make([]MetricSeries, 0, len(result))
	for _, ss := range result {
		samples := make([]MetricSample, 0, len(grid))
		for i, t := range grid {
			if !ss.has[i] {
				continue
			}
			samples = append(samples, MetricSample{Timestamp: t, Value: ss.values[i]})
		}
		out = append(out, MetricSeries{
			MetricName: ss.metricName,
			Labels:     ss.labels,
			Samples:    samples,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return labelSignature(mergeSig(out[i].MetricName, out[i].Labels)) < labelSignature(mergeSig(out[j].MetricName, out[j].Labels))
	})
	return out, nil
}

func mergeSig(metricName string, labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	out["__name__"] = metricName
	return out
}

func stepGrid(start, end time.Time, step time.Duration) []time.Time {
	var grid []time.Time
	for t := start; !t.After(end); t = t.Add(step) {
		grid = append(grid, t)
	}
	return grid
}

func evalExpr(ctx context.Context, s store.Storer, expr PromExpr, grid []time.Time, step time.Duration) (map[string]*stepSeries, error) {
	switch e := expr.(type) {
	case *VectorSelector:
		return evalSelector(ctx, s, e, grid, step)
	case *CallExpr:
		return evalCall(ctx, s, e, grid, step)
	case *AggExpr:
		return evalAgg(ctx, s, e, grid, step)
	case *PromBinaryExpr:
		return evalBinary(ctx, s, e, grid, step)
	case *NumberLiteral:
		return nil, &ParseError{Msg: "a bare number is not a valid top-level query"}
	default:
		return nil, &ParseError{Msg: "unrecognized expression node"}
	}
}

func fetchRawSeries(ctx context.Context, s store.Storer, metricName string, matchers []LabelMatcher, start, end time.Time) (map[string]*rawSeries, error) {
	whereSQL, args, err := CompileSelector(matchers)
	if err != nil {
		return nil, err
	}
	rows, err := s.QueryMetricPoints(ctx, metricName, whereSQL, args, start, end)
	if err != nil {
		return nil, err
	}
	out := map[string]*rawSeries{}
	for _, r := range rows {
		sig := labelSignature(r.Labels)
		rs, ok := out[sig]
		if !ok {
			rs = &rawSeries{labels: r.Labels}
			out[sig] = rs
		}
		rs.points = append(rs.points, point{ts: r.Timestamp, val: r.Value})
	}
	for _, rs := range out {
		sort.Slice(rs.points, func(i, j int) bool { return rs.points[i].ts.Before(rs.points[j].ts) })
	}
	return out, nil
}

// evalSelector resamples a bare vector selector onto grid: each step's value
// is the most recent raw point within (step-step_size, step] — a simple
// last-value-carried-forward-within-one-step-window resample, consistent
// with how evalCall windows rate().
func evalSelector(ctx context.Context, s store.Storer, sel *VectorSelector, grid []time.Time, step time.Duration) (map[string]*stepSeries, error) {
	if sel.RangeDur != nil {
		return nil, &ParseError{Msg: "a range-vector selector (e.g. metric_name[5m]) is only valid as rate()'s argument"}
	}
	if len(grid) == 0 {
		return map[string]*stepSeries{}, nil
	}

	raw, err := fetchRawSeries(ctx, s, sel.MetricName, sel.Matchers, grid[0].Add(-step), grid[len(grid)-1])
	if err != nil {
		return nil, err
	}

	out := make(map[string]*stepSeries, len(raw))
	for sig, rs := range raw {
		ss := &stepSeries{
			metricName: sel.MetricName,
			labels:     rs.labels,
			values:     make([]float64, len(grid)),
			has:        make([]bool, len(grid)),
		}
		for i, t := range grid {
			windowStart := t.Add(-step)
			var (
				foundVal float64
				foundAny bool
			)
			for _, pt := range rs.points {
				if pt.ts.After(t) {
					break
				}
				if pt.ts.After(windowStart) {
					foundVal, foundAny = pt.val, true
				}
			}
			if foundAny {
				ss.values[i] = foundVal
				ss.has[i] = true
			}
		}
		out[sig] = ss
	}
	return out, nil
}

// evalCall evaluates rate(): for each grid step, the rate is computed over
// the points falling in (step-RangeDur, step], using the standard
// counter-reset-aware algorithm (a decrease between consecutive points is
// treated as a reset back to zero, per Prometheus's own rate() semantics),
// divided by the wall-clock duration actually covered by those points. A
// step with fewer than two points in its window produces no sample (has[i]
// stays false) rather than a misleading zero.
// evalCall does not use the outer step grid's interval — rate()'s window is
// governed entirely by the selector's own [RangeDur], not the sampling
// step — so the step parameter is intentionally unused here (kept for a
// uniform eval*(ctx, store, expr, grid, step) signature across evalSelector/
// evalCall/evalAgg/evalBinary, all dispatched identically from evalExpr).
func evalCall(ctx context.Context, s store.Storer, call *CallExpr, grid []time.Time, _ time.Duration) (map[string]*stepSeries, error) {
	if call.Func != "rate" {
		return nil, &ParseError{Msg: fmt.Sprintf("unsupported function %q — only rate() is supported in this PromQL subset", call.Func)}
	}
	sel := call.Arg
	if sel.RangeDur == nil {
		return nil, &ParseError{Msg: "rate() requires a range-vector argument, e.g. rate(metric_name[5m])"}
	}
	if len(grid) == 0 {
		return map[string]*stepSeries{}, nil
	}
	rangeDur := *sel.RangeDur

	raw, err := fetchRawSeries(ctx, s, sel.MetricName, sel.Matchers, grid[0].Add(-rangeDur), grid[len(grid)-1])
	if err != nil {
		return nil, err
	}

	out := make(map[string]*stepSeries, len(raw))
	for sig, rs := range raw {
		ss := &stepSeries{
			labels: rs.labels,
			values: make([]float64, len(grid)),
			has:    make([]bool, len(grid)),
		}
		for i, t := range grid {
			windowStart := t.Add(-rangeDur)
			var window []point
			for _, pt := range rs.points {
				if pt.ts.After(windowStart) && !pt.ts.After(t) {
					window = append(window, pt)
				}
			}
			if rate, ok := computeRate(window); ok {
				ss.values[i] = rate
				ss.has[i] = true
			}
		}
		out[sig] = ss
	}
	return out, nil
}

func computeRate(window []point) (float64, bool) {
	if len(window) < 2 {
		return 0, false
	}
	var increase float64
	for i := 1; i < len(window); i++ {
		delta := window[i].val - window[i-1].val
		if delta < 0 {
			// Counter reset: the counter dropped (process restart, etc.) —
			// treat it as if it reset to zero then rose to the new value,
			// per Prometheus's own rate() semantics.
			delta = window[i].val
		}
		increase += delta
	}
	seconds := window[len(window)-1].ts.Sub(window[0].ts).Seconds()
	if seconds <= 0 {
		return 0, false
	}
	return increase / seconds, true
}

// evalAgg groups its inner expression's step-value series by the projected
// `by(...)` label subset (or into a single group with no labels, if `by` is
// empty/absent — full aggregation, matching PromQL's `sum(x)` without a
// `by` clause) and combines them per step using Op, ignoring missing values.
func evalAgg(ctx context.Context, s store.Storer, agg *AggExpr, grid []time.Time, step time.Duration) (map[string]*stepSeries, error) {
	inner, err := evalExpr(ctx, s, agg.Expr, grid, step)
	if err != nil {
		return nil, err
	}

	n := len(grid)
	type accumulator struct {
		labels map[string]string
		sum    []float64
		count  []int
		min    []float64
		max    []float64
		has    []bool
	}
	accs := map[string]*accumulator{}

	for _, ss := range inner {
		projected := projectLabels(ss.labels, agg.By)
		sig := labelSignature(projected)
		acc, ok := accs[sig]
		if !ok {
			acc = &accumulator{
				labels: projected,
				sum:    make([]float64, n),
				count:  make([]int, n),
				min:    make([]float64, n),
				max:    make([]float64, n),
				has:    make([]bool, n),
			}
			for i := range acc.min {
				acc.min[i] = math.Inf(1)
				acc.max[i] = math.Inf(-1)
			}
			accs[sig] = acc
		}
		for i := range n {
			if !ss.has[i] {
				continue
			}
			v := ss.values[i]
			acc.sum[i] += v
			acc.count[i]++
			if v < acc.min[i] {
				acc.min[i] = v
			}
			if v > acc.max[i] {
				acc.max[i] = v
			}
			acc.has[i] = true
		}
	}

	out := make(map[string]*stepSeries, len(accs))
	for sig, acc := range accs {
		res := &stepSeries{labels: acc.labels, values: make([]float64, n), has: make([]bool, n)}
		for i := range n {
			if !acc.has[i] {
				continue
			}
			switch agg.Op {
			case "sum":
				res.values[i] = acc.sum[i]
			case "avg":
				res.values[i] = acc.sum[i] / float64(acc.count[i])
			case "max":
				res.values[i] = acc.max[i]
			case "min":
				res.values[i] = acc.min[i]
			}
			res.has[i] = true
		}
		out[sig] = res
	}
	return out, nil
}

func projectLabels(labels map[string]string, by []string) map[string]string {
	out := make(map[string]string, len(by))
	for _, k := range by {
		if v, ok := labels[k]; ok {
			out[k] = v
		}
	}
	return out
}

// evalBinary supports only *scalar* arithmetic in this subset — one operand
// must be a *NumberLiteral. Vector-to-vector binary operations would need a
// label-matching join semantic (PromQL's `on`/`ignoring`/`group_left`) that
// is explicitly out of scope for this deliberately small subset; asking for
// one returns a clear *ParseError rather than guessing a join.
func evalBinary(ctx context.Context, s store.Storer, be *PromBinaryExpr, grid []time.Time, step time.Duration) (map[string]*stepSeries, error) {
	leftLit, leftIsLit := be.Left.(*NumberLiteral)
	rightLit, rightIsLit := be.Right.(*NumberLiteral)

	switch {
	case rightIsLit && !leftIsLit:
		left, err := evalExpr(ctx, s, be.Left, grid, step)
		if err != nil {
			return nil, err
		}
		return applyScalar(left, be.Op, rightLit.Value, false)
	case leftIsLit && !rightIsLit:
		right, err := evalExpr(ctx, s, be.Right, grid, step)
		if err != nil {
			return nil, err
		}
		return applyScalar(right, be.Op, leftLit.Value, true)
	case leftIsLit && rightIsLit:
		v, err := applyOp(be.Op, leftLit.Value, rightLit.Value)
		if err != nil {
			return nil, err
		}
		ss := &stepSeries{labels: map[string]string{}, values: make([]float64, len(grid)), has: make([]bool, len(grid))}
		for i := range ss.values {
			ss.values[i] = v
			ss.has[i] = true
		}
		return map[string]*stepSeries{"": ss}, nil
	default:
		return nil, &ParseError{Msg: "vector-to-vector binary operations are not supported in this PromQL subset — one side must be a scalar number"}
	}
}

// applyScalar applies op to every present value of every series in in,
// against the fixed scalar. scalarIsLeft controls operand order for
// non-commutative ops (- and /).
func applyScalar(in map[string]*stepSeries, op string, scalar float64, scalarIsLeft bool) (map[string]*stepSeries, error) {
	out := make(map[string]*stepSeries, len(in))
	for sig, ss := range in {
		res := &stepSeries{labels: ss.labels, values: make([]float64, len(ss.values)), has: make([]bool, len(ss.has))}
		for i, v := range ss.values {
			if !ss.has[i] {
				continue
			}
			var (
				result float64
				err    error
			)
			if scalarIsLeft {
				result, err = applyOp(op, scalar, v)
			} else {
				result, err = applyOp(op, v, scalar)
			}
			if err != nil {
				return nil, err
			}
			res.values[i] = result
			res.has[i] = true
		}
		out[sig] = res
	}
	return out, nil
}

func applyOp(op string, a, b float64) (float64, error) {
	switch op {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/":
		if b == 0 {
			return 0, &ParseError{Msg: "division by zero"}
		}
		return a / b, nil
	default:
		return 0, &ParseError{Msg: fmt.Sprintf("unsupported binary operator %q", op)}
	}
}

// labelSignature builds a canonical, order-independent string key for a
// label set so raw points/step-series belonging to the same distinct label
// combination group together correctly regardless of map iteration order.
func labelSignature(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(labels[k])
		sb.WriteByte(0)
	}
	return sb.String()
}
