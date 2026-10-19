package dbcore

import "testing"

// Every connection the pool opens carries the pragmas, not only the first one:
// the idle connection is dropped so the next query has to open a new one.
func TestEveryConnectionCarriesTheForeignKeyPragma(t *testing.T) {
	setupTestDB(t)
	DB.SetMaxIdleConns(0)
	defer DB.SetMaxIdleConns(2)

	for i := 0; i < 3; i++ {
		var foreignKeys, busy int
		var journal string
		if err := DB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if err := DB.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if err := DB.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 || busy != 5000 || journal != "wal" {
			t.Fatalf("connection %d: foreign_keys=%d busy_timeout=%d journal_mode=%s", i, foreignKeys, busy, journal)
		}
	}
}

func TestAPathWithAQuestionMarkIsRefused(t *testing.T) {
	if err := openConn("/tmp/a?b.db"); err == nil {
		t.Fatal("a path holding a question mark was opened")
	}
}
