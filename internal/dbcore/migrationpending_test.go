package dbcore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationPending(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "none.db")
	if pending, err := MigrationPending(missing); err != nil || pending {
		t.Errorf("missing file: pending=%v err=%v, want false and no error", pending, err)
	}

	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if pending, err := MigrationPending(empty); err != nil || pending {
		t.Errorf("empty file: pending=%v err=%v, want false and no error", pending, err)
	}

	for _, c := range []struct {
		version int
		want    bool
	}{{schemaVersion - 1, true}, {schemaVersion, false}, {schemaVersion + 1, false}} {
		path := filepath.Join(dir, fmt.Sprintf("v%d.db", c.version))
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", c.version)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TABLE t (x INTEGER)"); err != nil {
			t.Fatal(err)
		}
		db.Close()
		pending, err := MigrationPending(path)
		if err != nil || pending != c.want {
			t.Errorf("schema %d: pending=%v err=%v, want %v", c.version, pending, err, c.want)
		}
	}
}
