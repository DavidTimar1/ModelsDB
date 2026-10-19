package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The rebuild compiles and runs code that lives in the tagged commit
// (`go run ./cmd/minifyassets` runs it outright, and every build runs the
// package init functions of the modules it compiles). That code must never be
// able to read the signing seed, the maintainer's gh login or the process
// environment, so every such command runs inside bubblewrap with a root built
// from nothing: no network, an empty home folder, and only the folders the
// build needs.

// sandboxPath is the only place bubblewrap is looked for, never PATH.
const sandboxPath = "/usr/bin/bwrap"

// sandboxBinary finds bubblewrap. A variable so a test can replace it. An empty
// answer makes publish refuse: it never builds outside the sandbox.
var sandboxBinary = func() string {
	if fi, err := os.Stat(sandboxPath); err == nil && fi.Mode().IsRegular() {
		return sandboxPath
	}
	return ""
}

// sandboxSpec describes one sandbox: what the child may see and the exact
// environment it gets. Nothing here comes from os.Environ().
type sandboxSpec struct {
	Bwrap    string   // the bubblewrap program
	Home     string   // the home folder, replaced by an empty tmpfs
	Writable []string // folders bound read-write at their own path
	ReadOnly []string // folders bound read-only at their own path
	Hidden   []string // files overlaid with an empty file, wherever they are
	Env      []string // NAME=VALUE pairs, the whole environment of the child
}

// commandContext builds the bubblewrap command that runs name with args in dir,
// and stops the sandbox when ctx ends. The root is a whitelist: only /proc, /dev,
// a private /tmp, an empty home and the listed folders exist, so a file outside
// them (the key file included) is not there to open. Hidden files are overlaid
// last, so a key that lies inside a bound folder reads as empty too.
func (s sandboxSpec) commandContext(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	a := []string{"--unshare-all", "--die-with-parent", "--new-session", "--clearenv",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
	if s.Home != "" {
		a = append(a, "--tmpfs", s.Home)
	}
	for _, p := range s.ReadOnly {
		a = append(a, "--ro-bind", p, p)
	}
	for _, p := range s.Writable {
		a = append(a, "--bind", p, p)
	}
	for _, p := range s.Hidden {
		a = append(a, "--ro-bind", "/dev/null", p)
	}
	for _, kv := range s.Env {
		if name, val, ok := strings.Cut(kv, "="); ok {
			a = append(a, "--setenv", name, val)
		}
	}
	a = append(a, "--chdir", dir, "--", name)
	a = append(a, args...)
	cmd := exec.CommandContext(ctx, s.Bwrap, a...)
	cmd.Env = []string{} // bubblewrap itself gets nothing either
	return cmd
}

// hiddenSpellings returns every path under which the key file could be opened:
// the name as given, made absolute, and with symlinks resolved.
func hiddenSpellings(keyPath string) []string {
	if keyPath == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if abs, err := filepath.Abs(keyPath); err == nil {
		add(abs)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			add(real)
		}
	}
	return out
}

// sandboxFor builds the sandbox of one rebuild. goroot is the absolute Go root
// from goRoot; the job's src and out folders are the only writable source
// folders; cache is a private build cache and modcache is the module cache
// filled before the sandbox started.
func sandboxFor(job buildJob, goroot, cache, modcache string) (sandboxSpec, error) {
	bwrap := sandboxBinary()
	if bwrap == "" {
		return sandboxSpec{}, sandboxMissing()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return sandboxSpec{}, fmt.Errorf("cannot tell which folder is your home folder, so it cannot be hidden: %w", err)
	}
	return sandboxSpec{
		Bwrap:    bwrap,
		Home:     home,
		ReadOnly: []string{goroot, modcache},
		Writable: []string{job.Src, job.OutDir, cache},
		Hidden:   hiddenSpellings(job.KeyPath),
		Env: []string{
			"PATH=" + filepath.Join(goroot, "bin") + ":/usr/bin:/bin",
			"HOME=/tmp",
			"GOCACHE=" + cache,
			"GOMODCACHE=" + modcache,
			"GOPATH=/tmp/gopath",
			"GOPROXY=off",
		},
	}, nil
}
