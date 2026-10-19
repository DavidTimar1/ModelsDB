package dbcore

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The environment variables that turn a re-exec of this test binary into the
// child half of TestOpenDBRefusesBelowFloor. shared.Fatalf calls os.Exit, so
// the refusal itself has to run in a subprocess: a direct call in the test
// process would end the whole test run rather than let it check what was left
// behind.
const (
	floorCheckChildEnv = "MODELSDB_TEST_FLOORCHECK_CHILD"
	floorCheckPathEnv  = "MODELSDB_TEST_FLOORCHECK_PATH"
)

// TestOpenDBRefusesBelowFloor proves the floor check runs before anything is
// opened for writing: a database stamped below schemaFloor is left BYTE FOR
// BYTE unchanged, and OpenDB never returns - it exits instead, naming the
// path, the found version and the floor in its message.
func TestOpenDBRefusesBelowFloor(t *testing.T) {
	if os.Getenv(floorCheckChildEnv) == "1" {
		runFloorCheckChild()
		return
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "modelsdb.db")

	// Build a real database at a version below the floor, then hash it, so the
	// test can prove the refusal changed nothing.
	if err := buildBelowFloorDB(t, dbPath, schemaFloor-1); err != nil {
		t.Fatalf("build a below-floor database: %v", err)
	}
	before, err := hashFile(dbPath)
	if err != nil {
		t.Fatalf("hash the database before the refusal: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestOpenDBRefusesBelowFloor$")
	cmd.Env = append(os.Environ(), floorCheckChildEnv+"=1", floorCheckPathEnv+"="+dbPath)
	out, cmdErr := cmd.CombinedOutput()

	if cmdErr == nil {
		t.Fatalf("OpenDB should have exited non-zero on a below-floor database; output:\n%s", out)
	}
	output := string(out)
	if !strings.Contains(output, dbPath) {
		t.Errorf("the refusal should name the database path; output:\n%s", output)
	}
	if !strings.Contains(output, "version 32") {
		t.Errorf("the refusal should name the found version (32); output:\n%s", output)
	}
	if !strings.Contains(output, "docs/migration.md") {
		t.Errorf("the refusal should point at docs/migration.md; output:\n%s", output)
	}
	if !strings.Contains(output, bridgeReleaseURL) {
		t.Errorf("the refusal should name the conversion release %s; output:\n%s", bridgeReleaseURL, output)
	}
	if strings.Contains(output, "605449693c51563ffb12497350d52f7f15873954") {
		t.Errorf("the refusal names a commit that is not in the public repository; output:\n%s", output)
	}

	after, err := hashFile(dbPath)
	if err != nil {
		t.Fatalf("hash the database after the refusal: %v", err)
	}
	if before != after {
		t.Fatalf("the below-floor database changed on a refused open: before=%s after=%s", before, after)
	}
	// No sidecar or new file was created next to it either.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the refusal left files behind: %v, want only modelsdb.db", names)
	}
}

// runFloorCheckChild is the child half: it calls OpenDB directly, which is
// expected to exit the process through the floor refusal rather than return.
func runFloorCheckChild() {
	OpenDB(os.Getenv(floorCheckPathEnv))
	panic("OpenDB returned; the floor check did not refuse a below-floor database")
}

// buildBelowFloorDB creates a real SQLite file with the floor's own table set
// and stamps user_version below the floor, so the refusal is exercised against
// a genuine database rather than an empty file (which checkFloor treats as a
// fresh install, not an old one).
func buildBelowFloorDB(t *testing.T, path string, version int) error {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(coreSchemaSQL); err != nil {
		return err
	}
	// PRAGMA user_version takes no placeholders; version here is this test's own
	// int, never anything a caller supplies.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return nil
}

func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return string(sum[:]), nil
}
