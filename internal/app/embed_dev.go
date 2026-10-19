//go:build !minified

package app

import "embed"

// staticFS embeds internal/app/static/ directly - the human-authored,
// unminified app.js/style.css alongside the vendored jQuery/DataTables
// bundles. This is the default build (plain `go build`/`go run`, no tags),
// so every dev workflow (go run ./cmd/modelsdb, the dev wrapper scripts)
// always serves readable source with no minification step involved.
//
//go:embed static/*
var staticFS embed.FS

// staticFSSubdir names the directory staticFS embeds, so server.go can
// fs.Sub() into it without caring which build this is.
const staticFSSubdir = "static"
