package dbcore

import (
	"database/sql"
	"fmt"
	"modelsdb/internal/shared"
	"os"
)

// schemaFloor is the oldest schema version this build still opens. A database
// below it needs an upgrade step no migration here carries any more - see
// docs/migration.md in the repository - so it is refused rather than guessed
// at.
const schemaFloor = 33

// bridgeReleaseURL is the public pre-release holding the one-time conversion
// version, the last build that carries every migration below the floor. It is
// published unsigned and never as the latest release, so no install's update
// check sees it.
const bridgeReleaseURL = "https://github.com/DavidTimar1/ModelsDB/releases/tag/bridge-8.11.4"

// checkFloor refuses a database below schemaFloor, and NEVER opens it for
// writing to find out: it reads PRAGMA user_version through a second,
// read-only connection, so the file is left exactly as it was found. It exits
// the process on a refusal, and it never returns on any path that would leave
// OpenDB going on to open the file read-write.
//
// A missing or empty file is not an old database - a pre-versioning database
// and a brand-new one both read user_version 0, so existence is the only
// honest way to tell them apart, and it is checked first.
func checkFloor(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return // fresh install; nothing to refuse
	}
	v, err := readVersionReadOnly(path)
	if err != nil {
		shared.Fatalf("ModelsDB could not read the schema version of %s, and has changed nothing in it: %v", path, err)
		return
	}
	if v < schemaFloor {
		shared.Fatalf("%s", belowFloorMessage(path, v))
	}
}

// readVersionReadOnly reads PRAGMA user_version through a connection opened
// mode=ro, so the file is never written to find out. It is used both by
// checkFloor (before OpenDB touches anything) and by RestoreBackup (before a
// candidate is copied over the live database).
//
// A failed read-only open is returned as an error rather than retried
// read-write or read off the file's header bytes: under WAL the header's
// version can lag the real one, so the header cannot be trusted.
func readVersionReadOnly(path string) (int, error) {
	ro, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("open %s read-only: %w", path, err)
	}
	defer ro.Close()
	var v int
	if err := ro.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("read PRAGMA user_version of %s: %w", path, err)
	}
	return v, nil
}

// MigrationPending reports whether the database file at path is below the
// schema this build expects, so opening it would migrate it. It reads the
// version through a read-only connection and changes nothing. A missing or empty
// file has nothing to migrate.
func MigrationPending(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return false, nil
	}
	v, err := readVersionReadOnly(path)
	if err != nil {
		return false, err
	}
	return v < schemaVersion, nil
}

// belowFloorMessage is the exact text a below-floor refusal prints, so a
// stranger holding an old database knows what happened and what to do about
// it before anything of theirs is touched.
func belowFloorMessage(path string, v int) string {
	return fmt.Sprintf(`ModelsDB cannot open this database, and has changed nothing in it.

  Database:       %s
  Its schema:     version %d
  This build:     version %d at the oldest, %d now

A database older than version %d needs one upgrade step this build no
longer carries. Nothing has been read from it, written to it, or moved.

What to do, in order:
  1. Stop ModelsDB. Copy modelsdb.db, modelsdb.db-wal and modelsdb.db-shm
     together into a safe folder. All three, while ModelsDB is not running.
  2. Download the conversion version from
     %s
     and run its backup command once against your data folder. It carries
     the database forward and needs no network.
  3. Start this version against the same data folder.

Your view settings are not carried over, and the cells of columns this
version no longer has are dropped. Every value you typed is kept.

The steps in full: https://github.com/DavidTimar1/ModelsDB/blob/main/docs/migration.md
`, path, v, schemaFloor, schemaVersion, schemaFloor, bridgeReleaseURL)
}
