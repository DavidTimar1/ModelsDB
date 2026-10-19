package app

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runStart runs `start` against a sandbox and returns its output and exit code.
func runStart(t *testing.T, s sandbox) (out string, code int) {
	t.Helper()
	c := exec.Command(modelsdbBinary(t), s.args("start")...)
	c.Env = childEnv(s.dir)
	b, err := c.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return string(b), 0
	case errors.As(err, &exitErr):
		return string(b), exitErr.ExitCode()
	default:
		t.Fatalf("cannot run start: %v", err)
		return "", -1
	}
}

// A background start on a database the build refuses must say so, print the
// refusal the foreground start prints, exit non-zero, and change nothing in the
// file. Before this, `start` printed "Started" and exited 0 while the child died
// within a second and wrote the refusal only to the log file.
func TestBackgroundStartOnARefusedDatabaseReportsTheRefusal(t *testing.T) {
	s := newSandbox(t)
	dbPath := filepath.Join(s.dir, "modelsdb.db")
	old, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("cannot create the old database: %v", err)
	}
	for _, stmt := range []string{"CREATE TABLE models (name TEXT)", "PRAGMA user_version=5"} {
		if _, err := old.Exec(stmt); err != nil {
			t.Fatalf("cannot build the schema 5 fixture (%s): %v", stmt, err)
		}
	}
	old.Close()
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("cannot read the fixture: %v", err)
	}

	out, code := runStart(t, s)

	if code == 0 {
		t.Errorf("start exited 0 on a refused database:\n%s", out)
	}
	for _, want := range []string{"did not start", "ModelsDB cannot open this database", "version 5", "docs/migration.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("start output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Started ModelsDB") {
		t.Errorf("start claimed success on a refused database:\n%s", out)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("the refused database changed (read error: %v)", err)
	}
	if _, ok := health(s.port); ok {
		t.Errorf("something answers on port %d after a refused start", s.port)
	}
}

// A start on a good (here empty) data folder still returns with its success
// line, exits 0, and does not wait for the catalog refresh: the health route is
// still answering "starting" or "ok" afterwards, and `stop` ends the server.
func TestBackgroundStartOnAGoodDatabaseSucceedsPromptly(t *testing.T) {
	s := newSandbox(t)

	began := time.Now()
	out, code := runStart(t, s)
	took := time.Since(began)

	if code != 0 || !strings.Contains(out, "Started ModelsDB in the background") {
		t.Fatalf("start did not report success (exit %d):\n%s", code, out)
	}
	t.Cleanup(func() { runControl(t, s, "stop") })
	if took > backgroundWatch+10*time.Second {
		t.Errorf("start took %v, far longer than the %v watch", took, backgroundWatch)
	}
	if _, ok := health(s.port); !ok {
		t.Errorf("nothing answers on port %d after a successful start", s.port)
	}
}

// logSince returns only what the log gained, and drops the logger's stamp.
func TestLogSinceReturnsTheNewTextWithoutTheStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modelsdb.log")
	old := "2026/10/01 10:00:00 a.go:1: earlier run\n"
	fresh := "2026/10/19 12:00:00 floorcheck.go:42: refused\n  second line\n"
	if err := os.WriteFile(path, []byte(old+fresh), 0o644); err != nil {
		t.Fatal(err)
	}
	got := logSince(path, int64(len(old)))
	if want := "refused\n  second line"; got != want {
		t.Errorf("logSince = %q, want %q", got, want)
	}
	// A log rotated below the offset is read from its start.
	if got := logSince(path, 1<<20); !strings.Contains(got, "earlier run") {
		t.Errorf("a rotated log was not read from its start: %q", got)
	}
	if got := logSince(filepath.Join(t.TempDir(), "none.log"), 0); got != "" {
		t.Errorf("a missing log gave %q", got)
	}
}
