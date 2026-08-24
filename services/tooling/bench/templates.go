package main

import (
	"embed"
	"html/template"
)

// templatesFS embeds the html/template sources under templates/ into the
// compiled binary — mirrors services/tooling/garage/templates.go exactly,
// including the reasoning in its doc comment for why each page is its own
// *template.Template rather than one shared set (base.html's "content"
// block, and each page's "title" block, are deliberately reused names
// across pages).
//
//go:embed templates/*.html
var templatesFS embed.FS

var (
	dashboardTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/dashboard.html",
	))
	workflowListTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/workflow_list.html",
	))
	workflowDetailTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/workflow_detail.html",
	))
	workflowFormTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/workflow_form.html",
	))
	// pluginSearchTemplate parses only workflow_form.html, not base.html --
	// it renders the "plugin-search-results" block defined there (the
	// search sidebar's htmx-swapped fragment), mirroring
	// runDetailFragmentTemplate's identical reasoning below.
	pluginSearchTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/workflow_form.html",
	))
	settingsTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/settings.html",
	))
	runDetailTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/run_detail.html",
	))
	// runDetailFragmentTemplate parses only run_detail.html, not base.html --
	// it renders the "run-detail-fragment" block defined there (the part of
	// the page htmx swaps on refresh), not the "content"/"base" wrapper. See
	// templates/run_detail.html's own comment for the block layout.
	runDetailFragmentTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/run_detail.html",
	))
	notFoundTemplate = template.Must(template.ParseFS(templatesFS,
		"templates/base.html",
		"templates/not_found.html",
	))
)
