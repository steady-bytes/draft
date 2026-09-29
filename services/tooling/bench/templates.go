package main

import (
	"embed"

	draftui "github.com/steady-bytes/draft/tools/draft-ui"
)

// templatesFS embeds the page templates under templates/. The shared frame (shell, rail, topbar,
// status bar) and the component partials come from the design system module (tools/draft-ui), so
// there is no base.html here any more — each page only defines its own `content` (and, when it needs
// them, `topbar` controls or a `head`). Mirrors services/tooling/foundry/templates.go.
//
//go:embed templates/*.html
var templatesFS embed.FS

// kit is Bench's view of the shared design system: its identity and rail. The rail's counts are set
// per request (ui.go's frame), and the Apps block is derived from the request host
// (bench.draft.localhost ↔ foundry.draft.localhost) with direct-port fallbacks for a plain
// `localhost:9300`.
//
// The mockups also list "Webhook secrets" under Settings; no such page exists, so it is left out
// rather than drawn as a dead link (the design-system plan's D12).
var kit = draftui.New(draftui.App{
	Kind: draftui.KindBench,
	Name: "bench",
	Rail: []draftui.NavSection{
		{Label: "Workflows", Items: []draftui.NavItem{
			{Label: "Overview", Path: "/", Exact: true},
			{Label: "All workflows", Path: "/workflows"},
			{Label: "+ New workflow", Path: "/workflows/new", Action: true, Exact: true},
		}},
		{Label: "Runs", Items: []draftui.NavItem{
			{Label: "Recent runs", Path: "/runs", Exact: true},
			{Label: "Failed · 24h", Path: "/runs?status=failed"},
		}},
		{Label: "Settings", Items: []draftui.NavItem{
			{Label: "Plugin registries", Path: "/settings"},
		}},
	},
	Fallback: []draftui.NavItem{
		draftui.AppItem(draftui.KindFoundry, "http://localhost:9301/"),
		draftui.AppItem(draftui.KindBlueprint, "http://localhost:2221/"),
		draftui.AppItem(draftui.KindBeacon, "http://localhost:2222/"),
		draftui.AppItem(draftui.KindLineman, "http://localhost:9307/"),
	},
})

// Each page is its own template set (see draftui.Kit.Page): `content` is deliberately redefined per
// page, so parsing them into one shared set would let the last-parsed page win for all. The
// partial files (runs_table, workflow_table, run_fragment, editor_feedback, plugin_results) define
// only named blocks, and are parsed alongside every page that uses them.
var (
	overviewTemplate = kit.Page(templatesFS,
		"templates/workflow_table.html",
		"templates/overview.html",
	)
	workflowTableTemplate = kit.Fragment(templatesFS,
		"templates/workflow_table.html",
	)
	runsTemplate = kit.Page(templatesFS,
		"templates/runs_table.html",
		"templates/runs.html",
	)
	runTemplate = kit.Page(templatesFS,
		"templates/run_fragment.html",
		"templates/run.html",
	)
	runFragmentTemplate = kit.Fragment(templatesFS,
		"templates/run_fragment.html",
	)
	workflowDetailTemplate = kit.Page(templatesFS,
		"templates/runs_table.html",
		"templates/workflow_detail.html",
	)
	editorTemplate = kit.Page(templatesFS,
		"templates/editor_feedback.html",
		"templates/plugin_results.html",
		"templates/editor.html",
	)
	editorFeedbackTemplate = kit.Fragment(templatesFS,
		"templates/editor_feedback.html",
	)
	pluginResultsTemplate = kit.Fragment(templatesFS,
		"templates/plugin_results.html",
	)
	settingsTemplate = kit.Page(templatesFS,
		"templates/settings.html",
	)
	notFoundTemplate = kit.Page(templatesFS,
		"templates/not_found.html",
	)
)
