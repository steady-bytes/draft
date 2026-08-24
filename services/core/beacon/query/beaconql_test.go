package query

import (
	"testing"
	"time"

	"github.com/steady-bytes/draft/services/core/beacon/store"
)

func mustParse(t *testing.T, filter string) Expr {
	t.Helper()
	expr, err := ParseBeaconQL(filter)
	if err != nil {
		t.Fatalf("ParseBeaconQL(%q) returned error: %v", filter, err)
	}
	return expr
}

func TestParseBeaconQL_Empty(t *testing.T) {
	expr, err := ParseBeaconQL("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expr != nil {
		t.Fatalf("expected nil expr for empty filter, got %#v", expr)
	}
}

func TestParseBeaconQL_MalformedReturnsParseError(t *testing.T) {
	// Syntax errors are caught by the parser itself.
	syntaxCases := []string{
		"severity_text",               // missing operator
		"severity_text = ",            // missing value
		"severity_text = 'error' AND", // dangling AND
		"severity_text == \"error\"",  // invalid operator
		"severity_text LIKE 5",        // LIKE requires a string pattern
	}
	for _, c := range syntaxCases {
		if _, err := ParseBeaconQL(c); err == nil {
			t.Errorf("ParseBeaconQL(%q) expected an error, got nil", c)
		} else if _, ok := err.(*ParseError); !ok {
			t.Errorf("ParseBeaconQL(%q) expected *ParseError, got %T: %v", c, err, err)
		}
	}

	// Semantic errors (unknown field, misuse of map fields) are syntactically
	// valid so the parser accepts them — they surface as a *ParseError from
	// Compile instead, which is what rpc.go's toConnectError checks for the
	// client-facing "clear parse error, not a ClickHouse error" guarantee.
	semanticCases := []string{
		`attributes = "x"`,    // map field without index
		`unknown_field = "x"`, // unknown column
	}
	for _, c := range semanticCases {
		expr, err := ParseBeaconQL(c)
		if err != nil {
			t.Errorf("ParseBeaconQL(%q) unexpectedly failed to parse: %v", c, err)
			continue
		}
		if _, _, err := Compile(expr); err == nil {
			t.Errorf("Compile(ParseBeaconQL(%q)) expected an error, got nil", c)
		} else if _, ok := err.(*ParseError); !ok {
			t.Errorf("Compile(ParseBeaconQL(%q)) expected *ParseError, got %T: %v", c, err, err)
		}
	}
}

func TestCompile_SimpleComparisonIsParameterized(t *testing.T) {
	expr := mustParse(t, `severity = "error" AND service_name = "beacon"`)
	sql, args, err := Compile(expr)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	wantSQL := `(severity_text = ?) AND (service_name = ?)`
	if sql != wantSQL {
		t.Fatalf("sql = %q, want %q", sql, wantSQL)
	}
	if len(args) != 2 || args[0] != "error" || args[1] != "beacon" {
		t.Fatalf("args = %#v, want [error beacon]", args)
	}
	// The literal values must never appear in the SQL string itself.
	if containsSubstr(sql, "error") || containsSubstr(sql, "beacon") {
		t.Fatalf("sql leaked a literal value: %q", sql)
	}
}

func TestCompile_MapIndexBindsKeyAsParam(t *testing.T) {
	expr := mustParse(t, `attributes["route"] LIKE "/api/%"`)
	sql, args, err := Compile(expr)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	wantSQL := `attributes[?] LIKE ?`
	if sql != wantSQL {
		t.Fatalf("sql = %q, want %q", sql, wantSQL)
	}
	if len(args) != 2 || args[0] != "route" || args[1] != "/api/%" {
		t.Fatalf("args = %#v, want [route /api/%%]", args)
	}
}

func TestCompile_InAndNotIn(t *testing.T) {
	expr := mustParse(t, `severity_number IN (9, 10, 11)`)
	sql, args, err := Compile(expr)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	if sql != "severity_number IN (?, ?, ?)" {
		t.Fatalf("sql = %q", sql)
	}
	if len(args) != 3 {
		t.Fatalf("args = %#v", args)
	}

	expr = mustParse(t, `service_name NOT IN ("a", "b")`)
	sql, _, err = Compile(expr)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	if sql != "service_name NOT IN (?, ?)" {
		t.Fatalf("sql = %q", sql)
	}
}

func TestMatches_MirrorsCompileSemantics(t *testing.T) {
	row := store.LogRow{
		Timestamp:      time.Now(),
		ServiceName:    "beacon",
		SeverityText:   "error",
		SeverityNumber: 17,
		Attributes:     map[string]string{"route": "/api/logs"},
	}
	other := store.LogRow{
		Timestamp:      time.Now(),
		ServiceName:    "catalyst",
		SeverityText:   "info",
		SeverityNumber: 9,
		Attributes:     map[string]string{"route": "/health"},
	}

	expr := mustParse(t, `severity = "error" AND service_name = "beacon" AND attributes["route"] LIKE "/api/%"`)

	if !Matches(expr, row) {
		t.Fatalf("expected matching row to match")
	}
	if Matches(expr, other) {
		t.Fatalf("expected non-matching row to be excluded")
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
