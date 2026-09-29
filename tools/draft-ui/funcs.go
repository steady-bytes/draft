package draftui

import (
	"fmt"
	"html/template"
	"strings"
)

// Tag, StatusDot, Cell and friends are the small view models the partials take. Build them with
// the template functions (`{{template "d-tag" (tag "err" "Failed")}}`) or in Go.

// Tag is an outlined chip in a meaning colour. Tone: primary, ca, bp, fs, sv, err, warn or
// "" / quiet.
type Tag struct {
	Tone  string
	Text  string
	Solid bool
	Title string
}

// Class is the full class list.
func (t Tag) Class() string {
	c := "d-tag"
	switch t.Tone {
	case "", "quiet":
		c += " d-tag--quiet"
	default:
		c += " d-tag--" + t.Tone
	}
	if t.Solid {
		c += " d-tag--solid"
	}
	return c
}

// StatusDot is a dot and a label. Kind: "" / ok, info, warn, err, idle.
type StatusDot struct {
	Kind string
	Live bool
	Text string
}

// Class is the full class list.
func (s StatusDot) Class() string {
	c := "d-status"
	switch s.Kind {
	case "info", "warn", "err", "idle":
		c += " d-status--" + s.Kind
	}
	if s.Live {
		c += " d-status--live"
	}
	return c
}

// Cell is one cell of a strip. State: "" (ok), err, warn, run, none, skip.
type Cell struct {
	State   string
	Title   string
	Current bool
}

// Class is the cell's state classes.
func (c Cell) Class() string {
	var parts []string
	if c.State != "" && c.State != "ok" {
		parts = append(parts, "is-"+c.State)
	}
	if c.Current {
		parts = append(parts, "is-cur")
	}
	return strings.Join(parts, " ")
}

// Strip is a row of small cells. Size: "" (8×16), xs, stretch, tall.
type Strip struct {
	Cells []Cell
	Size  string
	Label string
}

// Class is the strip's class list.
func (s Strip) Class() string {
	if s.Size == "" {
		return "d-strip"
	}
	return "d-strip d-strip--" + s.Size
}

// Stat is a stat tile. DeltaGood: "good", "bad" or "" (neutral).
type Stat struct {
	Label     string
	Value     string
	Unit      string
	Delta     string
	DeltaGood string
	Spark     []float64
	SparkTone string
	// ValueTone colours the value: err, warn, primary …
	ValueTone string
}

// SparkPath is the SVG path for the tile's sparkline.
func (s Stat) SparkPath() string { return SparkPath(s.Spark, 96, 24) }

// DeltaClass is the delta's class list.
func (s Stat) DeltaClass() string {
	switch s.DeltaGood {
	case "good":
		return "d-delta is-good"
	case "bad":
		return "d-delta is-bad"
	}
	return "d-delta"
}

// Empty is an empty state.
type Empty struct {
	Title, Text string
	Compact     bool
}

// Code is a highlighted code block. Lang: json, yaml, http or "" (plain).
type Code struct {
	Lang string
	Text string
	Wrap bool
	// Bare drops the block's own border and background, for use inside a panel.
	Bare bool
	ID   string
}

// Class is the block's class list.
func (c Code) Class() string {
	class := "d-code"
	if c.Wrap {
		class += " d-code--wrap"
	}
	if c.Bare {
		class += " d-code--bare"
	}
	return class
}

// HTML is the highlighted, escaped content.
func (c Code) HTML() template.HTML { return Highlight(c.Lang, c.Text) }

func funcMap() template.FuncMap {
	return template.FuncMap{
		"asset":    Asset,
		"tag":      func(tone, text string) Tag { return Tag{Tone: tone, Text: text} },
		"tagt":     func(tone, text, title string) Tag { return Tag{Tone: tone, Text: text, Title: title} },
		"solidtag": func(tone, text string) Tag { return Tag{Tone: tone, Text: text, Solid: true} },
		"status":   func(kind, text string) StatusDot { return StatusDot{Kind: kind, Text: text} },
		"live":     func(kind, text string) StatusDot { return StatusDot{Kind: kind, Live: true, Text: text} },
		"strip":    func(cells []Cell, size string) Strip { return Strip{Cells: cells, Size: size} },
		"empty":    func(title, text string) Empty { return Empty{Title: title, Text: text} },
		"code":     func(lang, text string) Code { return Code{Lang: lang, Text: text} },
		"wrapcode": func(lang, text string) Code { return Code{Lang: lang, Text: text, Wrap: true} },
		// panelcode is a bare block with an id (a copy button targets it), for use inside a panel.
		"panelcode":    func(lang, id, text string) Code { return Code{Lang: lang, Text: text, Bare: true, ID: id} },
		"hl":           Highlight,
		"loginCard":    LoginCard,
		"registerCard": RegisterCard,
		"spark":        func(values []float64) string { return SparkPath(values, 96, 24) },
		"dict":         dict,
		"count":        FormatCount,
		"seq": func(n int) []int {
			out := make([]int, n)
			for i := range out {
				out[i] = i
			}
			return out
		},
		"add": func(a, b int) int { return a + b },
	}
}

// dict builds a map from alternating keys and values, for passing several arguments to a partial:
// {{template "x" (dict "Title" "a" "Body" .Text)}}.
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}
