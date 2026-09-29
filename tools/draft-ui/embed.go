// Package draftui is the Go (server-rendered) side of the Draft design system.
//
// It embeds the compiled assets from dist/ (the same CSS the Dioxus clients use) and the shared
// html/template shell and partials from templates/, so Bench and Foundry render the same frame and
// components as the WASM apps without a Rust dependency. The class contract (`d-*`) is the
// interface between the two renderers; see README.md.
package draftui

import "embed"

// The compiled assets. dist/ is generated (`cargo run --features cli --bin draft-ui-css`) and
// committed because go:embed cannot build CSS.
//
//go:embed dist/draft.css dist/fonts.css dist/draft.js dist/boot.js dist/fonts dist/vendor
var assetsFS embed.FS

//go:embed templates/*.html
var templatesFS embed.FS
