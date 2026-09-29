package draftui

import (
	"net"
	"net/http"
	"strings"
)

// Kind identifies a Draft app. It drives the glyph and its colour, like the Rust `AppKind`.
type Kind string

const (
	KindBlueprint Kind = "blueprint"
	KindCatalyst  Kind = "catalyst"
	KindFuse      Kind = "fuse"
	KindBeacon    Kind = "beacon"
	KindBench     Kind = "bench"
	KindFoundry   Kind = "foundry"
	KindLineman   Kind = "lineman"
	KindService   Kind = "service"
)

// Code is the two-letter glyph (Bp, Bc, Bn, …).
func (k Kind) Code() string {
	switch k {
	case KindBlueprint:
		return "Bp"
	case KindCatalyst:
		return "Ca"
	case KindFuse:
		return "Fs"
	case KindBeacon:
		return "Bc"
	case KindBench:
		return "Bn"
	case KindFoundry:
		return "Fd"
	case KindLineman:
		return "Lm"
	default:
		return "Sv"
	}
}

// GlyphClass is the `d-glyph--*` colour modifier ("" = the default slate glyph). As in the
// mockups: Blueprint and Lineman violet, Catalyst and Beacon blue, Fuse amber, Bench and Foundry
// slate.
func (k Kind) GlyphClass() string {
	switch k {
	case KindBlueprint, KindLineman:
		return "d-glyph--bp"
	case KindCatalyst, KindBeacon:
		return "d-glyph--ca"
	case KindFuse:
		return "d-glyph--fs"
	default:
		return ""
	}
}

// linkable are the apps that have a UI of their own, in the order the rail lists them.
var linkable = []Kind{KindBeacon, KindBench, KindBlueprint, KindFoundry, KindLineman}

func label(k Kind) string {
	s := string(k)
	return strings.ToUpper(s[:1]) + s[1:]
}

// AppLinks returns the rail's Apps block for a request.
//
// When the page is served from an app subdomain (`bench.draft.localhost:10000`, the
// service-UI-subdomains convention) the other apps are its siblings: same scheme, same port, first
// label swapped. Otherwise (a direct `localhost:9300`) the app's configured Fallback is used.
func (k *Kit) AppLinks(r *http.Request) []NavItem {
	host, port := splitHostPort(r.Host)
	labels := strings.Split(host, ".")
	if len(labels) < 3 || net.ParseIP(host) != nil {
		return append([]NavItem(nil), k.app.Fallback...)
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	domain := strings.Join(labels[1:], ".")
	var out []NavItem
	for _, kind := range linkable {
		if kind == k.app.Kind {
			continue
		}
		url := scheme + "://" + string(kind) + "." + domain
		if port != "" {
			url += ":" + port
		}
		out = append(out, NavItem{
			Label:      label(kind),
			Path:       url + "/",
			External:   true,
			Glyph:      kind.Code(),
			GlyphClass: kind.GlyphClass(),
		})
	}
	return out
}

func splitHostPort(hostport string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p
	}
	return hostport, ""
}

// AppItem builds a rail link to another app, e.g. for an app's Fallback list.
func AppItem(kind Kind, url string) NavItem {
	return NavItem{Label: label(kind), Path: url, External: true, Glyph: kind.Code(), GlyphClass: kind.GlyphClass()}
}
