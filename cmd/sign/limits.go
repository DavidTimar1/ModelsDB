package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Everything run from the tagged commit, and the module download its go.mod
// asks for, is hostile input: it may run forever, fill the disk, or print
// text meant to fool the terminal or an agent reading it. These limits and
// the output filter keep that to a refused publish with a fixed message.

// The values are variables so a test can shorten them.
var (
	// downloadTimeout bounds `go mod download`. A cold download plus a cold
	// build of this app takes a few minutes, so 15 minutes leaves several times
	// the room for a slow link.
	downloadTimeout = 15 * time.Minute
	// stepTimeout bounds each sandboxed command (the asset minifier and each
	// build), on the same reasoning.
	stepTimeout = 15 * time.Minute
	// maxWorkBytes caps the work folder (source, module cache, build cache and
	// output). A full cold run for this app stays under 1 GB, so 4 GiB leaves
	// several times the room; the cap exists to stop a hostile go.mod filling a
	// disk, or a RAM-backed temp folder.
	maxWorkBytes int64 = 4 << 30
	// watchEvery is how often the work folder's size is measured.
	watchEvery = 2 * time.Second
)

// maxShownOutput is the most of a child's output kept, and the most shown.
const maxShownOutput = 4096

// Fixed refusal messages: nothing from the child is in them.
var (
	errTimedOut = errors.New("the command ran past its time limit and was stopped; nothing was signed")
	errTooBig   = errors.New("the build's work folder grew past its size limit and the command was stopped; nothing was signed")
)

// cleanOutput makes a child's output safe to print: control and format
// characters (escape sequences, bidi overrides) are dropped except newline and
// tab, and the text is cut to maxShownOutput bytes.
func cleanOutput(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || (!unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) && r != unicode.ReplacementChar) {
			b.WriteRune(r)
		}
		if b.Len() >= maxShownOutput {
			b.WriteString("\n... (output cut)")
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// capWriter keeps the first limit bytes written and drops the rest, so a child
// that prints without end cannot fill memory.
type capWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if room := w.limit - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// dirSize adds up the sizes of the files under dir; unreadable entries count as zero.
func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// runLimited runs the command that newCmd builds under a time limit, and stops
// it if the folder watch grows past maxWorkBytes. It returns the child's output
// already cleaned by cleanOutput.
func runLimited(timeout time.Duration, watch string, newCmd func(ctx context.Context) *exec.Cmd) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := newCmd(ctx)
	out := &capWriter{limit: 4 * maxShownOutput}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = 5 * time.Second

	var tooBig bool
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(watchEvery)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if dirSize(watch) > maxWorkBytes {
					mu.Lock()
					tooBig = true
					mu.Unlock()
					cancel()
					return
				}
			}
		}
	}()
	err := cmd.Run()
	close(done)

	mu.Lock()
	big := tooBig
	mu.Unlock()
	switch {
	case big:
		return "", errTooBig
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", errTimedOut
	}
	return cleanOutput(out.buf.String()), err
}
