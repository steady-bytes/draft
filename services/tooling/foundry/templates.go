package main

import (
	"embed"

	draftui "github.com/steady-bytes/draft/tools/draft-ui"
)

// templatesFS embeds the page templates under templates/. The shared frame (shell, rail, topbar,
// status bar) and the component partials come from the design system module (tools/draft-ui), so
// there is no base.html here any more — each page only defines its own `content` (and, if it
// needs page-local CSS, `head`).
//
//go:embed templates/*.html
var templatesFS embed.FS

// kit is Foundry's view of the shared design system: its identity and rail.
//
// The rail's Maintainers section is added per request (it is derived from the data), and the Apps
// block is derived from the request host (bench.draft.localhost ↔ foundry.draft.localhost) with
// direct-port fallbacks for a plain `localhost:9301`.
var kit = draftui.New(draftui.App{
	Kind: draftui.KindFoundry,
	Name: "foundry",
	Rail: []draftui.NavSection{
		{Label: "Catalog", Items: []draftui.NavItem{
			{Label: "All plugins", Path: "/", Exact: true},
			{Label: "Recently published", Path: "/?sort=newest"},
		}},
	},
	Fallback: []draftui.NavItem{
		draftui.AppItem(draftui.KindBench, "http://localhost:9300/"),
		draftui.AppItem(draftui.KindBlueprint, "http://localhost:2221/"),
	},
})

// Each page is its own template set (see draftui.Kit.Page): `content` is deliberately redefined
// per page, so parsing them into one shared set would let the last-parsed page win for all.
// card_grid.html's "card-grid" block is the one truly shared define: it is part of the full
// catalog page and, alone, the htmx-swapped search fragment.
var (
	catalogTemplate = kit.Page(templatesFS,
		"templates/card_grid.html",
		"templates/catalog.html",
	)
	cardGridTemplate = kit.Fragment(templatesFS,
		"templates/card_grid.html",
	)
	detailTemplate = kit.Page(templatesFS,
		"templates/detail.html",
	)
	notFoundTemplate = kit.Page(templatesFS,
		"templates/not_found.html",
	)
)
