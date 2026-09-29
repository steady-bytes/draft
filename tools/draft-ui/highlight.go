package draftui

import (
	"html/template"
	"strconv"
	"strings"
)

// Span is one run of text and its token class ("" = unstyled). The tokenizer is a port of the
// Rust `data::highlight`; both are checked against the same golden fixtures
// (tests/fixtures/highlight.json).
type Span struct {
	Class string
	Text  string
}

// Highlight renders code as escaped HTML with `tk-*` token spans. lang: json, yaml, http, or
// anything else for plain text.
func Highlight(lang, text string) template.HTML {
	var b strings.Builder
	for _, s := range Tokens(lang, text) {
		esc := template.HTMLEscapeString(s.Text)
		if s.Class == "" {
			b.WriteString(esc)
		} else {
			b.WriteString(`<span class="` + s.Class + `">` + esc + `</span>`)
		}
	}
	return template.HTML(b.String()) //nolint:gosec // every text run is escaped above
}

// Tokens splits code into styled runs, merging neighbours with the same class.
func Tokens(lang, text string) []Span {
	var spans []Span
	switch strings.ToLower(lang) {
	case "json":
		spans = tokJSON(text)
	case "yaml", "yml":
		spans = tokYAML(text)
	case "http":
		spans = tokHTTP(text)
	default:
		spans = []Span{{Text: text}}
	}
	return mergeSpans(spans)
}

func mergeSpans(in []Span) []Span {
	out := make([]Span, 0, len(in))
	for _, s := range in {
		if s.Text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Class == s.Class {
			out[n-1].Text += s.Text
			continue
		}
		out = append(out, s)
	}
	return out
}

func isAlpha(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// JSON ------------------------------------------------------------------------------------------

func tokJSON(text string) []Span {
	rs := []rune(text)
	var out []Span
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case c == '"':
			start := i
			i++
			for i < len(rs) && rs[i] != '"' {
				if rs[i] == '\\' {
					i++
				}
				i++
			}
			if i+1 < len(rs) {
				i++
			} else {
				i = len(rs)
			}
			j := i
			for j < len(rs) && (rs[j] == ' ' || rs[j] == '\t' || rs[j] == '\n' || rs[j] == '\r') {
				j++
			}
			class := "tk-str"
			if j < len(rs) && rs[j] == ':' {
				class = "tk-key"
			}
			out = append(out, Span{class, string(rs[start:i])})
		case c == '-' || isDigit(c):
			start := i
			i++
			for i < len(rs) && (isDigit(rs[i]) || rs[i] == '.' || rs[i] == 'e' || rs[i] == 'E' || rs[i] == '+' || rs[i] == '-') {
				i++
			}
			out = append(out, Span{"tk-num", string(rs[start:i])})
		case isAlpha(c):
			start := i
			for i < len(rs) && isAlpha(rs[i]) {
				i++
			}
			w := string(rs[start:i])
			class := ""
			if w == "true" || w == "false" || w == "null" {
				class = "tk-val"
			}
			out = append(out, Span{class, w})
		default:
			out = append(out, Span{"", string(c)})
			i++
		}
	}
	return out
}

// YAML ------------------------------------------------------------------------------------------

func tokYAML(text string) []Span {
	var out []Span
	lines := strings.Split(text, "\n")
	for n, line := range lines {
		out = yamlLine(line, out)
		if n+1 < len(lines) {
			out = append(out, Span{"", "\n"})
		}
	}
	return out
}

func yamlLine(line string, out []Span) []Span {
	// A comment: `#` at the start or after whitespace, outside quotes.
	commentAt := -1
	var quote rune
	prevWS := true
	for idx, c := range line {
		if quote != 0 {
			if c == quote {
				quote = 0
			}
		} else if c == '"' || c == '\'' {
			quote = c
		} else if c == '#' && prevWS {
			commentAt = idx
			break
		}
		prevWS = c == ' ' || c == '\t'
	}
	code, comment := line, ""
	if commentAt >= 0 {
		code, comment = line[:commentAt], line[commentAt:]
	}

	trimmed := strings.TrimLeft(code, " \t")
	out = append(out, Span{"", code[:len(code)-len(trimmed)]})
	rest := trimmed
	if strings.HasPrefix(rest, "- ") {
		out = append(out, Span{"", "- "})
		rest = strings.TrimLeft(rest[2:], " ")
	}
	if colon := findKeyColon(rest); colon >= 0 {
		out = append(out, Span{"tk-key", rest[:colon]}, Span{"", ":"})
		out = yamlValue(rest[colon+1:], out)
	} else {
		out = yamlValue(rest, out)
	}
	if comment != "" {
		out = append(out, Span{"tk-com", comment})
	}
	return out
}

func findKeyColon(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ':':
			if i > 0 && (i+1 == len(s) || s[i+1] == ' ') {
				return i
			}
		case ' ', '{', '[', '"', '\'', ',':
			return -1
		}
	}
	return -1
}

func yamlValue(text string, out []Span) []Span {
	rs := []rune(text)
	var word []rune
	flow := 0
	flush := func() {
		if len(word) > 0 {
			out = append(out, classifyWord(string(word), flow > 0)...)
			word = word[:0]
		}
	}
	for i := 0; i < len(rs); {
		c := rs[i]
		if c == '$' && i+1 < len(rs) && rs[i+1] == '{' && len(word) == 0 {
			end := len(rs)
			for j := i; j < len(rs); j++ {
				if rs[j] == '}' {
					end = j + 1
					break
				}
			}
			out = append(out, Span{"tk-ph", string(rs[i:end])})
			i = end
			continue
		}
		if (c == '"' || c == '\'') && len(word) == 0 {
			end := len(rs)
			for j := i + 1; j < len(rs); j++ {
				if rs[j] == c {
					end = j + 1
					break
				}
			}
			out = append(out, Span{"", string(rs[i:end])})
			i = end
			continue
		}
		switch c {
		case ' ', '\t', ',', '{', '}', '[', ']':
			flush()
			if c == '{' || c == '[' {
				flow++
			} else if (c == '}' || c == ']') && flow > 0 {
				flow--
			}
			out = append(out, Span{"", string(c)})
		default:
			word = append(word, c)
		}
		i++
	}
	flush()
	return out
}

func classifyWord(w string, inFlow bool) []Span {
	if inFlow && strings.HasSuffix(w, ":") && len(w) > 1 {
		return []Span{{"tk-key", strings.TrimSuffix(w, ":")}, {"", ":"}}
	}
	class := ""
	switch {
	case strings.HasPrefix(w, "${"):
		class = "tk-ph"
	case strings.Contains(w, "://"):
		class = "tk-url"
	case isNumber(w):
		class = "tk-num"
	case w == "true" || w == "false" || w == "null" || w == "yes" || w == "no" || w == "~":
		class = "tk-val"
	}
	return []Span{{class, w}}
}

// isNumber mirrors Rust's `str::parse::<f64>` for the words YAML values contain.
func isNumber(w string) bool {
	if w == "" || strings.ContainsAny(w, " _") {
		return false
	}
	if _, err := strconv.ParseFloat(w, 64); err != nil {
		return false
	}
	lower := strings.ToLower(strings.TrimLeft(w, "+-"))
	// Rust accepts "inf", "infinity" and "nan"; Go's ParseFloat does too, but so would words like
	// "Infinity" in prose — keep it consistent by rejecting hex floats and underscores only.
	return !strings.HasPrefix(lower, "0x")
}

// HTTP ------------------------------------------------------------------------------------------

func tokHTTP(text string) []Span {
	var out []Span
	lines := strings.Split(text, "\n")
	inBody := false
	for n, line := range lines {
		switch {
		case n == 0:
			if rest, ok := strings.CutPrefix(line, "HTTP/"); ok {
				code := 0
				if f := strings.Fields(rest); len(f) > 1 {
					code, _ = strconv.Atoi(f[1])
				}
				class := ""
				if code >= 400 {
					class = "tk-err"
				}
				out = append(out, Span{class, line})
			} else if method, r, ok := strings.Cut(line, " "); ok {
				out = append(out, Span{"tk-key", method}, Span{"", " " + r})
			} else {
				out = append(out, Span{"", line})
			}
		case inBody:
			out = append(out, Span{"", line})
		case strings.TrimSpace(line) == "":
			inBody = true
			out = append(out, Span{"", line})
		default:
			if name, value, ok := strings.Cut(line, ":"); ok {
				out = append(out, Span{"tk-key", name}, Span{"", ":" + value})
			} else {
				out = append(out, Span{"", line})
			}
		}
		if n+1 < len(lines) {
			out = append(out, Span{"", "\n"})
		}
	}
	return out
}
