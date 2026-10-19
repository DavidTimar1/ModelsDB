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

// A rotation whose fresh file cannot be opened does not leave the writer holding
// a closed file: the failure is reported and a later write recovers.
func TestRotatingWriterRecoversWhenTheFreshFileCannotBeOpened(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "x.log")
	w, err := NewRotatingWriter(p, 30, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Write([]byte("0123456789012345678\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("0123456789012345678\n")); err == nil {
		t.Fatal("a write with no folder to write into reported success")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("after\n")); err != nil {
		t.Fatalf("the writer did not recover once the folder was back: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "after\n" {
		t.Errorf("log holds %q, want the line written after recovery", b)
	}
}

func TestRotatingWriterRefusesAWriteAfterClose(t *testing.T) {
	w, err := NewRotatingWriter(filepath.Join(t.TempDir(), "x.log"), 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	if _, err := w.Write([]byte("late")); err == nil {
		t.Error("a write after Close was accepted")
	}
}
