//go:build unix

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// linkHome points the home directory at a fresh folder and returns the folder
// the managed link lives in.
func linkHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".local", "bin")
}

func writeBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
}

// The program is commonly installed at the link path itself. Adding it to PATH
// must leave that file in place and runnable.
func TestInstallLinkNeverReplacesTheRunningBinary(t *testing.T) {
	bin := linkHome(t)
	exe := filepath.Join(bin, "modelsdb")
	writeBinary(t, exe)

	if err := installLink(exe); err == nil {
		t.Fatal("installLink replaced or accepted a regular file at the link path")
	}
	fi, err := os.Lstat(exe)
	if err != nil {
		t.Fatalf("the binary is gone: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the binary was replaced by a symlink")
	}
	if b, _ := os.ReadFile(exe); string(b) != "binary" {
		t.Fatalf("the binary's content changed: %q", b)
	}
}

func TestInstallLinkNeverReplacesAnyRegularFile(t *testing.T) {
	bin := linkHome(t)
	exe := filepath.Join(t.TempDir(), "modelsdb")
	writeBinary(t, exe)
	other := filepath.Join(bin, "modelsdb")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("the reader's own script"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := installLink(exe); err == nil {
		t.Fatal("a regular file at the link path was replaced")
	}
	if b, _ := os.ReadFile(other); string(b) != "the reader's own script" {
		t.Fatalf("the file's content changed: %q", b)
	}
}

func TestInstallLinkCreatesAndRefreshesTheManagedLink(t *testing.T) {
	linkHome(t)
	first := filepath.Join(t.TempDir(), "modelsdb")
	second := filepath.Join(t.TempDir(), "modelsdb")
	writeBinary(t, first)
	writeBinary(t, second)

	if err := installLink(first); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if on, dest := linkStatus(); !on || dest != first {
		t.Fatalf("link = %v -> %q, want a link to %q", on, dest, first)
	}
	if err := installLink(first); err != nil {
		t.Fatalf("installing the same target again: %v", err)
	}
	if err := installLink(second); err != nil {
		t.Fatalf("moving the link: %v", err)
	}
	if on, dest := linkStatus(); !on || dest != second {
		t.Fatalf("link = %v -> %q, want a link to %q", on, dest, second)
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("binary %s was removed: %v", p, err)
		}
	}
}

func TestInstallLinkLeavesAForeignLinkAlone(t *testing.T) {
	bin := linkHome(t)
	exe := filepath.Join(t.TempDir(), "modelsdb")
	writeBinary(t, exe)
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr/bin/true", filepath.Join(bin, "modelsdb")); err != nil {
		t.Fatal(err)
	}
	if err := installLink(exe); err == nil {
		t.Fatal("a symlink to something that is not a modelsdb binary was replaced")
	}
	if dest, _ := os.Readlink(filepath.Join(bin, "modelsdb")); dest != "/usr/bin/true" {
		t.Fatalf("the foreign link now points at %q", dest)
	}
}

func TestRemoveLinkNeverRemovesARegularFile(t *testing.T) {
	bin := linkHome(t)
	exe := filepath.Join(bin, "modelsdb")
	writeBinary(t, exe)
	if err := removeLink(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("removeLink removed the regular file: %v", err)
	}
}
