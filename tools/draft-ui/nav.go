package draftui

import (
	"strconv"
	"strings"
)

// NavItem is one rail link.
type NavItem struct {
	Label string
	// Path is an in-app path (/workflows) or, when External, a full URL.
	Path string
	// Count is shown in a small bordered number; empty = none.
	Count    string
	CountErr bool
	// External links to another Draft app: opens in a new tab and shows the ↗ marker.
	External bool
	// Action is the primary-coloured "+ New …" link.
	Action bool
	// Glyph is the two-letter kind chip before the label; GlyphClass its colour modifier.
	Glyph      string
	GlyphClass string
	// Exact highlights only on an exact path match (default: also for sub-paths).
	Exact bool
	// Current is set by Kit.NewPage from the request path.
	Current bool
}

// NavSection is a labelled group of rail links. A section with no items is not rendered.
type NavSection struct {
	Label string
	Items []NavItem
}

// IsCurrent reports whether the item is the current page for the request path. It mirrors the
// Rust `NavItem::is_current`: "/" matches only itself, and a path also matches its sub-paths
// unless Exact.
func (n NavItem) IsCurrent(current string) bool {
	if n.External {
		return false
	}
	path := strings.TrimRight(n.Path, "/")
	cur := current
	if i := strings.IndexAny(cur, "?#"); i >= 0 {
		cur = cur[:i]
	}
	cur = strings.TrimRight(cur, "/")
	if path == "" {
		return cur == ""
	}
	return cur == path || (!n.Exact && strings.HasPrefix(cur, path+"/"))
}

func cloneSections(in []NavSection) []NavSection {
	out := make([]NavSection, len(in))
	for i, s := range in {
		out[i] = NavSection{Label: s.Label, Items: append([]NavItem(nil), s.Items...)}
	}
	return out
}

func markCurrent(sections []NavSection, current string) {
	for i := range sections {
		for j := range sections[i].Items {
			sections[i].Items[j].Current = sections[i].Items[j].IsCurrent(current)
		}
	}
}

// FormatCount renders a counter with thousands separators: 4118 -> "4,118".
func FormatCount(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
