module github.com/home-cloud-io/docs

go 1.22.7

require (
	github.com/steady-bytes/draft/tools/draft-ui v0.0.0-00010101000000-000000000000 // indirect
	github.com/colinwilson/lotusdocs v0.1.0 // indirect
	github.com/gohugoio/hugo-mod-bootstrap-scss/v5 v5.20300.20200 // indirect
)

// The design system module, from the repo (there is no published version).
replace github.com/steady-bytes/draft/tools/draft-ui => ../../tools/draft-ui
