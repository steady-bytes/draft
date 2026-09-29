package draftui

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

// App describes one service: who it is and what its rail contains.
type App struct {
	Kind Kind
	// Name is the small text beside the wordmark (`bench`). Empty for Blueprint, whose brand is
	// just `{draft}`.
	Name string
	// Rail is the app's own navigation sections.
	Rail []NavSection
	// Fallback is the Apps block when the page is not on an app subdomain (a direct
	// `localhost:9300`), where sibling URLs cannot be derived from the host.
	Fallback []NavItem
}

// Title is the app name for <title>.
func (a App) Title() string {
	if a.Name != "" {
		return a.Name
	}
	return string(a.Kind)
}

// Status is the pill in the topbar ("1 run in flight").
type Status struct {
	// Class is the `d-status--*` modifier: "" (ok), "d-status--info", "--warn", "--err", "--idle".
	Class string
	Live  bool
	Text  string
}

// BarItem is one item of the status bar. Kind is "kv" (Label + Value), "hint" (a key and its
// text) or "text". ID, on a "kv" item, is put on the value's element so a script can update it
// (the editor's cursor position).
type BarItem struct {
	Kind, Label, Value string
	ID                 string
}

// KV, Hint and Text build status bar items.
func KV(label, value string) BarItem { return BarItem{Kind: "kv", Label: label, Value: value} }
func Hint(key, text string) BarItem  { return BarItem{Kind: "hint", Label: key, Value: text} }
func Text(text string) BarItem       { return BarItem{Kind: "text", Value: text} }

// KVID is KV with an id on the value element.
func KVID(id, label, value string) BarItem {
	return BarItem{Kind: "kv", Label: label, Value: value, ID: id}
}

// Page is the shell's data. Embed it in each page's view model:
//
//	type overviewPage struct {
//	    draftui.Page
//	    Workflows []workflowRow
//	}
//	data := overviewPage{Page: kit.NewPage(r, "Overview", "Bench", "Overview"), ...}
//	data.SetCount("/workflows", len(rows))
type Page struct {
	Title   string
	App     *App
	Current string
	Crumbs  []string
	Status  *Status
	// Left and Right are the status bar's two sides.
	Left, Right []BarItem
	// Rail is the app's navigation with Current marked. Counts are set with SetCount.
	Rail []NavSection
	// Apps is the shared Apps block.
	Apps []NavItem
	// Flush removes the main area's padding (full-bleed content).
	Flush bool
}

// SetCount sets the count on the rail item with this path.
func (p *Page) SetCount(path string, n int) { p.setCount(path, n, false) }

// SetErrCount is SetCount for a problem count (rendered in the error colour); zero clears it.
func (p *Page) SetErrCount(path string, n int) { p.setCount(path, n, true) }

func (p *Page) setCount(path string, n int, isErr bool) {
	for i := range p.Rail {
		for j := range p.Rail[i].Items {
			it := &p.Rail[i].Items[j]
			if it.Path == path && !it.External {
				it.Count, it.CountErr = FormatCount(n), isErr
				if isErr && n == 0 {
					it.Count, it.CountErr = "", false
				}
			}
		}
	}
}

// Kit renders an app's pages: it owns the parsed shell and partials and hands out per-page
// template sets, replacing each service's own base.html / static.go / templates.go.
type Kit struct {
	app   App
	funcs template.FuncMap
	base  *template.Template
}

// New parses the shared templates for app. It panics on a malformed embedded template, which is a
// programming error caught by the tests, like template.Must.
func New(app App) *Kit {
	k := &Kit{app: app, funcs: funcMap()}
	k.base = template.Must(template.New("draftui").Funcs(k.funcs).ParseFS(templatesFS, "templates/*.html"))
	return k
}

// App returns the app the kit was built for.
func (k *Kit) App() *App { return &k.app }

// NewPage builds the shell data for a request: rail with the current item marked, and the Apps
// block for the request's host.
func (k *Kit) NewPage(r *http.Request, title string, crumbs ...string) Page {
	rail := cloneSections(k.app.Rail)
	markCurrent(rail, r.URL.Path)
	return Page{
		Title:   title,
		App:     &k.app,
		Current: r.URL.Path,
		Crumbs:  crumbs,
		Rail:    rail,
		Apps:    k.AppLinks(r),
	}
}

// Page returns a template set for a full page: the shell, the partials and files, each of which
// defines `content` (and may define `head`). Execute the "shell" template.
//
// Every page gets its own set because `content` is deliberately redefined per page — parsing them
// into one shared set would let the last page parsed win for all.
func (k *Kit) Page(fsys fs.FS, files ...string) *template.Template {
	t := template.Must(k.base.Clone())
	return template.Must(t.ParseFS(fsys, files...))
}

// Fragment returns a template set for an htmx fragment: the partials and files, without the
// shell.
func (k *Kit) Fragment(fsys fs.FS, files ...string) *template.Template {
	t := template.Must(template.New("draftui-fragment").Funcs(k.funcs).ParseFS(templatesFS, "templates/partials.html"))
	return template.Must(t.ParseFS(fsys, files...))
}

// Render executes the named template into a buffer first, so a template error becomes a clean 500
// instead of a half-written page.
func (k *Kit) Render(w http.ResponseWriter, t *template.Template, name string, data any) error {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return fmt.Errorf("render %s: %w", name, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(buf.Bytes())
	return err
}
