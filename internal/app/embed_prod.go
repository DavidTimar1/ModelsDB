//go:build minified

package app

import "embed"

// staticFS embeds internal/app/static-min/, the minified copy of static/
// that `go run ./cmd/minifyassets` generates. build.sh runs that command and
// then builds with `-tags minified` to produce the shipped binaries, so only
// a production build ever serves minified app.js/style.css; the repo's own
// internal/app/static/ sources stay untouched and readable.
//
//go:embed static-min/*
var staticFS embed.FS

// staticFSSubdir names the directory staticFS embeds, so server.go can
// fs.Sub() into it without caring which build this is.
const staticFSSubdir = "static-min"
