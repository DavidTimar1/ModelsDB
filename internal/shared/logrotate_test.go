package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingWriterCapsSizeAndBackups(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	w, err := NewRotatingWriter(p, 100, 2) // 100 bytes, keep 2 backups
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	line := []byte("0123456789\n") // 11 bytes
	for i := 0; i < 50; i++ {      // 550 bytes total -> forces several rotations
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() > 110 {
		t.Fatalf("active log not capped: size=%v err=%v", fi.Size(), err)
	}
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatalf("expected backup .1: %v", err)
	}
	if _, err := os.Stat(p + ".2"); err != nil {
		t.Fatalf("expected backup .2: %v", err)
	}
	if _, err := os.Stat(p + ".3"); err == nil {
		t.Fatal(".3 should not exist (maxFiles=2)")
	}
}
