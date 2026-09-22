package main

import (
	"embed"
	"html/template"
)

// templatesFS embeds the html/template sources under templates/ into the
// compiled binary, so the UI doesn't depend on a working directory relative
// to the source tree at runtime (mirrors staticFS in static.go).
//
//go:embed templates/*.html
var templatesFS embed.FS

// Each page is its own *template.Template combining templates/base.html
// (the shared layout: DaisyUI theme setup, htmx script tag, header — see
// its doc comment) with that one page's own {{define "content"}} block. A
// single combined template.Template covering every page isn't used because
// "title"/"content" are deliberately reused block names across pages (each
// page overrides them) — parsing them all into one shared set would let the
// last-parsed page silently win for every page. card_grid.html's
// "card-grid" block is the one truly shared define: it's parsed into both
// catalogTemplate (as part of the full page) and cardGridTemplate (rendered
// alone, as the htmx-swapped search fragment).
var (
	catalogTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/card_grid.html",
		"templates/catalog.html",
	))
	cardGridTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/card_grid.html",
	))
	detailTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/detail.html",
	))
	notFoundTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/not_found.html",
	))
)
