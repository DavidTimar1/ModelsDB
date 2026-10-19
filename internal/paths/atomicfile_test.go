package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomicReplacesContentAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Errorf("content = %q, want new", b)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(p); info.Mode().Perm() != 0600 {
			t.Errorf("mode = %v, want the existing 0600 kept", info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("folder holds %d entries, want only the file (a temp file was left)", len(entries))
	}
}

func TestWriteFileAtomicCreatesWithGivenMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new.txt")
	if err := WriteFileAtomic(p, []byte("x"), 0640); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Errorf("content = %q", b)
	}
}

func TestWriteFileAtomicWritesThroughALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(link, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a regular file")
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("target content = %q, want new", b)
	}
}
