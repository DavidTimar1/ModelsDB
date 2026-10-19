package dbcore

// The one-time generator behind the floor schema: run once, by hand, against a
// migrations slice temporarily cut to end at the floor version, and never run
// as part of an ordinary `go test ./...`.
//
// It does nothing at all unless MODELSDB_SCHEMADUMP_DIR names the folder to
// write into.
//
//	MODELSDB_SCHEMADUMP_DIR=/tmp/schemadump \
//	  go test ./internal/dbcore -run TestDumpSchemaForFloorGeneration -count=1
//
// The path must be ABSOLUTE. Go runs a test with the package's own directory
// as the working directory, so a relative one would land under internal/dbcore.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpSchemaForFloorGeneration(t *testing.T) {
	outDir := os.Getenv("MODELSDB_SCHEMADUMP_DIR")
	if outDir == "" {
		t.Skip("MODELSDB_SCHEMADUMP_DIR is not set, so there is nowhere to write the dump")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("make the output folder %s: %v", outDir, err)
	}

	dbPath := filepath.Join(t.TempDir(), "throwaway.db")
	if err := OpenDB(dbPath); err != nil {
		t.Fatalf("open and migrate a throwaway database at %s: %v", dbPath, err)
	}
	defer DB.Close()

	var version int
	if err := DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}

	dump, err := dumpCreateStatements(DB)
	if err != nil {
		t.Fatalf("dump sqlite_master: %v", err)
	}

	path := filepath.Join(outDir, fmt.Sprintf("schema-v%d.sql", version))
	if err := os.WriteFile(path, []byte(dump), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s (schema version %d, %d bytes)", path, version, len(dump))
}

// dumpCreateStatements returns every CREATE TABLE and CREATE INDEX statement
// sqlite_master holds, tables first, in the order they were created, and each
// table's own indexes directly after it - which is the order a human reads the
// schema in.
func dumpCreateStatements(db *sql.DB) (string, error) {
	rows, err := db.Query(`SELECT type, name, tbl_name, COALESCE(sql,'') FROM sqlite_master
		WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'
		ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END, tbl_name, rowid`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var typ, name, tbl, sqlText string
		if err := rows.Scan(&typ, &name, &tbl, &sqlText); err != nil {
			return "", err
		}
		b.WriteString(strings.TrimSpace(sqlText))
		b.WriteString(";\n\n")
	}
	return b.String(), rows.Err()
}
