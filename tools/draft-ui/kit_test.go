package draftui

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var testApp = App{
	Kind: KindBench,
	Name: "bench",
	Rail: []NavSection{
		{Label: "Workflows", Items: []NavItem{
			{Label: "Overview", Path: "/", Exact: true},
			{Label: "All workflows", Path: "/workflows"},
			{Label: "+ New workflow", Path: "/workflows/new", Action: true},
		}},
		{Label: "Empty", Items: nil},
	},
	Fallback: []NavItem{AppItem(KindFoundry, "http://localhost:9301/")},
}

var pageFS = fstest.MapFS{
	"templates/page.html":   {Data: []byte(`{{define "content"}}<h1>{{.Heading}}</h1>{{template "d-tag" (tag "err" "Failed")}}{{template "d-status" (live "info" "Running")}}{{end}}`)},
	"templates/frag.html":   {Data: []byte(`{{define "frag"}}{{template "d-empty" (empty "None" "nothing")}}{{end}}`)},
	"templates/editor.html": {Data: []byte(`{{define "topbar"}}<button id="tb-save">Save</button>{{end}}{{define "content"}}<p>editor</p>{{end}}`)},
}

type testPage struct {
	Page
	Heading string
}

func newReq(host, path string) *http.Request {
	r := httptest.NewRequest("GET", "http://"+host+path, nil)
	r.Host = host
	return r
}

func render(t *testing.T, k *Kit, r *http.Request, mutate func(*testPage)) string {
	t.Helper()
	tpl := k.Page(pageFS, "templates/page.html")
	data := testPage{Page: k.NewPage(r, "Overview", "Bench", "Overview"), Heading: "Hello"}
	if mutate != nil {
		mutate(&data)
	}
	w := httptest.NewRecorder()
	if err := k.Render(w, tpl, "shell", data); err != nil {
		t.Fatal(err)
	}
	return w.Body.String()
}

func mustContain(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %q in:\n%s", sub, s)
		}
	}
}

func TestShellStructureMatchesTheRustShell(t *testing.T) {
	out := render(t, New(testApp), newReq("localhost:9300", "/workflows"), nil)
	// The CSS contract for the responsive rail: checkbox first, then brand, topbar, rail, main,
	// status bar, scrim — the same order tests/ssr.rs asserts for the Rust shell.
	last := -1
	for _, name := range []string{"d-rail-toggle", "d-brand", "d-topbar", "d-rail", "d-main", "d-statusbar", "d-rail-scrim"} {
		at := strings.Index(out, `class="`+name+`"`)
		if at < 0 {
			t.Fatalf("missing class %q:\n%s", name, out)
		}
		if at < last {
			t.Fatalf("%q out of order", name)
		}
		last = at
	}
	mustContain(t, out,
		`data-theme="draft"`,
		`aria-label="Toggle navigation"`,
		`aria-label="Breadcrumb"`,
		`aria-label="Navigation"`,
		`aria-disabled="true"`, // the ⌘K palette is drawn but inert
		`<h1>Hello</h1>`,
		`<title>Overview · bench</title>`,
		`/static/draft/draft.css?v=`,
		`/static/draft/fonts.css?v=`,
		`/static/draft/htmx.min.js?v=`,
	)
}

func TestRailMarksTheCurrentItemAndHidesEmptySections(t *testing.T) {
	out := render(t, New(testApp), newReq("localhost:9300", "/workflows/crud-e2e"), nil)
	if n := strings.Count(out, `aria-current="page"`); n != 1 {
		t.Fatalf("aria-current appears %d times, want exactly 1:\n%s", n, out)
	}
	// /workflows/crud-e2e is a sub-path of "All workflows", not of the exact "/" Overview.
	if !strings.Contains(out, `href="/workflows" aria-current="page"`) {
		t.Errorf("All workflows should be current:\n%s", out)
	}
	if strings.Contains(out, ">Empty<") {
		t.Errorf("empty section rendered")
	}
	mustContain(t, out, `d-nav-item d-nav-item--action`)
}

func TestNavItemCurrentRules(t *testing.T) {
	cases := []struct {
		item NavItem
		cur  string
		want bool
	}{
		{NavItem{Path: "/"}, "/", true},
		{NavItem{Path: "/"}, "/gateway", false},
		{NavItem{Path: "/gateway"}, "/gateway", true},
		{NavItem{Path: "/gateway"}, "/gateway/core-blueprint-ui", true},
		{NavItem{Path: "/gateway"}, "/gateways", false},
		{NavItem{Path: "/gateway", Exact: true}, "/gateway/x", false},
		{NavItem{Path: "/query"}, "/query?q=1#top", true},
		{NavItem{Path: "http://x/", External: true}, "/", false},
	}
	for _, c := range cases {
		if got := c.item.IsCurrent(c.cur); got != c.want {
			t.Errorf("%+v current(%q) = %v, want %v", c.item, c.cur, got, c.want)
		}
	}
}

func TestCountsAndStatusBar(t *testing.T) {
	out := render(t, New(testApp), newReq("localhost:9300", "/"), func(p *testPage) {
		p.SetCount("/workflows", 4118)
		p.SetErrCount("/workflows/new", 2)
		p.Status = &Status{Class: "d-status--info", Live: true, Text: "1 run in flight"}
		p.Left = []BarItem{KV("Registry", "foundry")}
		p.Right = []BarItem{Hint("/", "search")}
	})
	mustContain(t, out,
		`<span class="d-count">4,118</span>`,
		`<span class="d-count d-count--err">2</span>`,
		`d-status d-status--info d-status--live">1 run in flight`,
		`Registry <b>foundry</b>`,
		`<span class="d-kbd">/</span> search`,
	)
}

func TestErrCountOfZeroClears(t *testing.T) {
	out := render(t, New(testApp), newReq("localhost:9300", "/"), func(p *testPage) { p.SetErrCount("/workflows", 0) })
	if strings.Contains(out, "d-count--err") {
		t.Errorf("zero problem count should not render")
	}
}

func TestAppLinksFromSubdomainOrFallback(t *testing.T) {
	k := New(testApp)
	sub := k.AppLinks(newReq("bench.draft.localhost:10000", "/"))
	var labels []string
	for _, l := range sub {
		labels = append(labels, l.Label)
		if !l.External || !strings.HasPrefix(l.Path, "http://") || !strings.HasSuffix(l.Path, ".draft.localhost:10000/") {
			t.Errorf("bad sibling link %+v", l)
		}
	}
	if strings.Join(labels, ",") != "Beacon,Blueprint,Foundry,Lineman" { // no Bench: that is us
		t.Errorf("labels = %v", labels)
	}
	if sub[0].Path != "http://beacon.draft.localhost:10000/" || sub[0].Glyph != "Bc" || sub[0].GlyphClass != "d-glyph--ca" {
		t.Errorf("beacon link = %+v", sub[0])
	}

	direct := k.AppLinks(newReq("localhost:9300", "/"))
	if len(direct) != 1 || direct[0].Path != "http://localhost:9301/" {
		t.Errorf("fallback links = %+v", direct)
	}
	if ip := k.AppLinks(newReq("127.0.0.1:9300", "/")); len(ip) != 1 {
		t.Errorf("an IP host must use the fallback: %+v", ip)
	}

	r := newReq("bench.example.com", "/")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := k.AppLinks(r)[0].Path; got != "https://beacon.example.com/" {
		t.Errorf("https link = %q", got)
	}
}

func TestPartials(t *testing.T) {
	k := New(testApp)
	out := render(t, k, newReq("localhost:9300", "/"), nil)
	mustContain(t, out, `<span class="d-tag d-tag--err">Failed</span>`, `<span class="d-status d-status--info d-status--live">Running</span>`)

	frag := k.Fragment(pageFS, "templates/frag.html")
	w := httptest.NewRecorder()
	if err := frag.ExecuteTemplate(w, "frag", nil); err != nil {
		t.Fatal(err)
	}
	mustContain(t, w.Body.String(), `<div class="d-empty"><b>None</b><span>nothing</span></div>`)
}

func TestStatAndStripPartials(t *testing.T) {
	k := New(testApp)
	tpl := template.Must(template.Must(k.base.Clone()).Parse(`{{define "t"}}{{template "d-stat" .Stat}}{{template "d-strip" (strip .Cells "xs")}}{{end}}`))
	w := httptest.NewRecorder()
	data := map[string]any{
		"Stat":  Stat{Label: "Pass rate", Value: "97.9", Unit: "%", Delta: "47 of 48", DeltaGood: "good", Spark: []float64{1, 3, 2}, ValueTone: "err"},
		"Cells": []Cell{{}, {State: "err", Current: true}, {State: "none"}},
	}
	if err := tpl.ExecuteTemplate(w, "t", data); err != nil {
		t.Fatal(err)
	}
	mustContain(t, w.Body.String(),
		`class="d-panel d-stat"`, `style="color:var(--err)"`, `<small>%</small>`, `class="d-delta is-good"`, `class="d-spark"`, `<path d="M0.0 `,
		`class="d-strip d-strip--xs"`, `<i class="is-err is-cur">`, `<i class="is-none">`,
	)
}

func TestStaticHandler(t *testing.T) {
	h := Static()
	srv := httptest.NewServer(h)
	defer srv.Close()

	for name, wantType := range map[string]string{
		"draft.css":               "text/css",
		"draft.js":                "text/javascript",
		"fonts.css":               "text/css",
		"htmx.min.js":             "text/javascript",
		"fonts/inter-latin.woff2": "font/woff2",
	} {
		req := httptest.NewRequest("GET", StaticPrefix+name, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("%s: status %d", name, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, wantType) {
			t.Errorf("%s: content type %q, want %q", name, ct, wantType)
		}
		if w.Header().Get("ETag") == "" || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: missing ETag/Cache-Control", name)
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s: empty body", name)
		}

		// A conditional request is answered with 304 and no body.
		req2 := httptest.NewRequest("GET", StaticPrefix+name, nil)
		req2.Header.Set("If-None-Match", w.Header().Get("ETag"))
		w2 := httptest.NewRecorder()
		h.ServeHTTP(w2, req2)
		if w2.Code != http.StatusNotModified || w2.Body.Len() != 0 {
			t.Errorf("%s: conditional request = %d with %d bytes", name, w2.Code, w2.Body.Len())
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", StaticPrefix+"nope.css", nil))
	if w.Code != 404 {
		t.Errorf("unknown asset = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", StaticPrefix+"../go.mod", nil))
	if w.Code != 404 {
		t.Errorf("path traversal must not reach the filesystem: %d", w.Code)
	}
}

func TestAssetURLCarriesAContentHash(t *testing.T) {
	a := Asset("draft.css")
	if !strings.HasPrefix(a, "/static/draft/draft.css?v=") || len(a) != len("/static/draft/draft.css?v=")+8 {
		t.Errorf("Asset = %q", a)
	}
}

func TestRenderTemplateErrorIsACleanFiveHundred(t *testing.T) {
	k := New(testApp)
	tpl := template.Must(template.New("x").Parse(`{{define "bad"}}{{.Missing.Field}}{{end}}`))
	w := httptest.NewRecorder()
	if err := k.Render(w, tpl, "bad", struct{}{}); err == nil {
		t.Fatal("want an error")
	}
	if w.Code != 500 {
		t.Errorf("status = %d", w.Code)
	}
}

func TestSparkPathMatchesRust(t *testing.T) {
	// Same expectations as viz::geom tests in Rust.
	if d := SparkPath([]float64{1, 3, 2}, 96, 24); !strings.HasPrefix(d, "M0.0 ") || strings.Count(d, "L") != 2 {
		t.Errorf("path = %q", d)
	}
	if d := SparkPath([]float64{5, 5, 5}, 96, 24); !strings.Contains(d, "12.0") {
		t.Errorf("flat series should be mid-height: %q", d)
	}
	if SparkPath(nil, 96, 24) != "" {
		t.Errorf("empty series")
	}
}

func TestFormatCount(t *testing.T) {
	for in, want := range map[int]string{0: "0", 999: "999", 4118: "4,118", 1234567: "1,234,567", -1500: "-1,500"} {
		if got := FormatCount(in); got != want {
			t.Errorf("FormatCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestKindGlyphsMatchTheRustAppKind(t *testing.T) {
	want := map[Kind][2]string{
		KindBlueprint: {"Bp", "d-glyph--bp"}, KindCatalyst: {"Ca", "d-glyph--ca"}, KindFuse: {"Fs", "d-glyph--fs"},
		KindBeacon: {"Bc", "d-glyph--ca"}, KindBench: {"Bn", ""}, KindFoundry: {"Fd", ""}, KindLineman: {"Lm", "d-glyph--bp"},
	}
	for k, w := range want {
		if k.Code() != w[0] || k.GlyphClass() != w[1] {
			t.Errorf("%s = %s %q, want %v", k, k.Code(), k.GlyphClass(), w)
		}
	}
}

func TestPageMayAddControlsToTheTopbar(t *testing.T) {
	k := New(testApp)
	r := newReq("localhost:9300", "/workflows/new")
	w := httptest.NewRecorder()
	data := testPage{Page: k.NewPage(r, "Editor", "Bench", "Editor")}
	data.Status = &Status{Class: "d-status--err", Text: "1 problem"}
	if err := k.Render(w, k.Page(pageFS, "templates/editor.html"), "shell", data); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	status, tb, cmd := strings.Index(out, "1 problem"), strings.Index(out, `id="tb-save"`), strings.Index(out, "d-cmd")
	if status < 0 || tb < status || cmd < tb {
		t.Fatalf("topbar controls must sit between the status pill and the palette button (%d %d %d):\n%s", status, tb, cmd, out)
	}
	// A page that defines no topbar block renders the shell unchanged.
	plain := render(t, k, r, nil)
	if strings.Contains(plain, "tb-save") {
		t.Fatalf("topbar block leaked into another page")
	}
}

func TestStatusBarValueCanCarryAnID(t *testing.T) {
	out := render(t, New(testApp), newReq("localhost:9300", "/"), func(p *testPage) {
		p.Left = []BarItem{KVID("wf-cursor", "Cursor", "1:1"), KV("YAML", "26 lines")}
	})
	mustContain(t, out, `<span>Cursor <b id="wf-cursor">1:1</b></span>`, `<span>YAML <b>26 lines</b></span>`)
}

func TestBareLayoutHasNoShell(t *testing.T) {
	k := New(App{Kind: KindService, Name: "golf"})
	files := fstest.MapFS{
		"templates/login.html": {Data: []byte(`{{define "content"}}{{template "auth-card" (loginCard "/login")}}{{end}}`)},
	}
	r := newReq("localhost:8080", "/login")
	page := k.NewPage(r, "Sign in", "Golf")
	w := httptest.NewRecorder()
	if err := k.Render(w, k.Page(files, "templates/login.html"), "bare", page); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	mustContain(t, out, `<title>Sign in · golf</title>`, `/static/draft/draft.css?v=`, `<main class="d-auth">`, `class="d-panel d-auth-card"`)
	for _, shell := range []string{"d-rail", "d-topbar", "d-statusbar", "d-brand"} {
		if strings.Contains(out, shell) {
			t.Errorf("a standalone page must not carry the app shell (%s):\n%s", shell, out)
		}
	}
}

func TestLoginCardHasItsFields(t *testing.T) {
	out := renderAuth(t, "loginCard", `"/login"`)
	mustContain(t, out,
		`action="/login"`, `method="post"`,
		`name="username"`, `autocomplete="username"`,
		`type="password" id="password" name="password" autocomplete="current-password" required`,
		`name="remember-me"`,
		`>Sign in</button>`, `href="/register"`,
	)
	if strings.Contains(out, "d-alert") {
		t.Errorf("no error was given:\n%s", out)
	}
}

func TestRegisterCardShowsTheAlreadyExistsError(t *testing.T) {
	out := renderAuth(t, "registerCard", `"/register" true`)
	mustContain(t, out,
		`class="d-alert d-alert--err" role="alert">That username is taken.`,
		`name="password-confirmation"`, `autocomplete="new-password"`,
		`>Create account</button>`, `href="/login"`,
	)
	if strings.Contains(out, "remember-me") {
		t.Errorf("registering has no remember-me")
	}
	if clean := renderAuth(t, "registerCard", `"/register" false`); strings.Contains(clean, "d-alert") {
		t.Errorf("no error without a clash:\n%s", clean)
	}
}

func TestAuthCardEscapesWhatItIsGiven(t *testing.T) {
	k := New(testApp)
	card := LoginCard(`/login?next="><script>`)
	card.Error = `<b>bad</b>`
	files := fstest.MapFS{"templates/p.html": {Data: []byte(`{{define "content"}}{{template "auth-card" .Card}}{{end}}`)}}
	tpl := k.Page(files, "templates/p.html")
	w := httptest.NewRecorder()
	data := struct {
		Page
		Card AuthCard
	}{Page: k.NewPage(newReq("localhost:1", "/login"), "Sign in"), Card: card}
	if err := k.Render(w, tpl, "bare", data); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if strings.Contains(out, "<script>") || strings.Contains(out, "<b>bad</b>") {
		t.Fatalf("unescaped markup:\n%s", out)
	}
}

func renderAuth(t *testing.T, fn, args string) string {
	t.Helper()
	k := New(App{Kind: KindService, Name: "golf"})
	files := fstest.MapFS{"templates/p.html": {Data: []byte(`{{define "content"}}{{template "auth-card" (` + fn + ` ` + args + `)}}{{end}}`)}}
	w := httptest.NewRecorder()
	if err := k.Render(w, k.Page(files, "templates/p.html"), "bare", k.NewPage(newReq("localhost:1", "/"), "Sign in")); err != nil {
		t.Fatal(err)
	}
	return w.Body.String()
}
