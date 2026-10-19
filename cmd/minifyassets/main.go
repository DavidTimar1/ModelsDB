// Command minifyassets regenerates internal/app/static-min/, the minified
// copy of internal/app/static/ that a production binary embeds. It reads the
// human-authored app.js and style.css and writes minified versions, plus a
// verbatim copy of every other file in static/ (the already-minified vendor
// bundles and the favicon), into static-min/.
//
// static-min/ is a build artifact: it is git-ignored, and only
// internal/app/embed_prod.go's //go:embed directive reads it, which is only
// compiled into a binary built with `-tags minified`. build.sh runs this
// command before that build. Plain `go run ./cmd/modelsdb` (dev mode) never
// runs this and never sees static-min/: it embeds internal/app/static/
// directly, unminified, via internal/app/embed_dev.go.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/js"
)

const (
	srcDir = "internal/app/static"
	dstDir = "internal/app/static-min"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "minifyassets: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	// Start from a clean directory so a file removed from static/ never
	// lingers as a stale copy in static-min/.
	if err := os.RemoveAll(dstDir); err != nil {
		return fmt.Errorf("clear %s: %w", dstDir, err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dstDir, err)
	}

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("application/javascript", js.Minify)

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := processFile(m, e.Name()); err != nil {
			return err
		}
	}
	return nil
}

// processFile minifies app.js and style.css (the project's own, human-authored
// sources) and copies every other file in static/ (bundled vendor JS/CSS,
// already minified upstream, plus the favicon) through unchanged.
func processFile(m *minify.M, name string) error {
	src := filepath.Join(srcDir, name)
	dst := filepath.Join(dstDir, name)

	switch name {
	case "app.js":
		return minifyFile(m, "application/javascript", src, dst)
	case "style.css":
		return minifyFile(m, "text/css", src, dst)
	default:
		return copyFile(src, dst)
	}
}

func minifyFile(m *minify.M, mediatype, src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	out, err := m.Bytes(mediatype, in)
	if err != nil {
		return fmt.Errorf("minify %s: %w", src, err)
	}
	if err := os.WriteFile(dst, out, 0644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	fmt.Printf("  -> %s: %d -> %d bytes\n", filepath.Base(src), len(in), len(out))
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return out.Close()
}
