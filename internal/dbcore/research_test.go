package dbcore

import (
	"fmt"
	"testing"
)

// allMigrations is the whole registry as the package declared it, captured
// before any test narrows the package variable through migrateThrough, so a
// test that migrates in two stages can name the second target.
var allMigrations = migrations

// migrateUpTo runs the registered migrations up to and including target, from
// the full registry rather than whatever an earlier stage of the same test
// left in place.
func migrateUpTo(t *testing.T, target int) {
	t.Helper()
	migrations = allMigrations
	migrateThrough(t, target)
}

// The two rank layouts a database can carry into v46: the one
// declareColumnSources wrote on a fresh install (hand at -1, the rest from 0)
// and the one v40 wrote on a migrated catalog (hand at 0, the rest from 1).
type rankLayout struct {
	name     string
	handRank int
	first    int
}

var (
	freshLayout    = rankLayout{"fresh", -1, 0}
	migratedLayout = rankLayout{"migrated", 0, 1}
)

// v45Column makes one post-v45 column (no namespace, triggers live) and
// declares sources at the given layout: hand at the layout's hand rank, and
// the automatic sources from its first rank in the order given. It answers
// the column's id.
func v45Column(t *testing.T, key string, layout rankLayout, withHand bool, automatic ...string) int64 {
	t.Helper()
	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at) VALUES (?,?,'text',?)`,
		key, key, NowStamp())
	if err != nil {
		t.Fatalf("insert column %s: %v", key, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if withHand {
		if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'hand',?)`, id, layout.handRank); err != nil {
			t.Fatal(err)
		}
	}
	for i, src := range automatic {
		if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)`, id, src, layout.first+i); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// putSourcedCell stores one cell under a source, with a fixed written_at so
// a test can see whether the stamp travelled.
func putSourcedCell(t *testing.T, columnID int64, model, source, value, writtenAt string) {
	t.Helper()
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
		VALUES (?,?,?,?,NULL,?)`, columnID, model, source, value, writtenAt); err != nil {
		t.Fatalf("insert %s cell on column %d for %q: %v", source, columnID, model, err)
	}
}

// sourcesByRank reads a column's declared sources in rank order.
func sourcesByRank(t *testing.T, columnID int64) []string {
	t.Helper()
	rows, err := DB.Query(`SELECT source FROM column_sources WHERE column_id=? ORDER BY rank ASC`, columnID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func countWhere(t *testing.T, where string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := DB.QueryRow("SELECT COUNT(*) FROM cells WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestResearchReplacesHandOnAFetchedColumnAndKeepsTheCount covers the core of
// v46 on both layouts: a hand cell on a column OpenRouter states becomes a
// research cell with the same text, the cell count is level, and the column
// no longer declares hand.
func TestResearchReplacesHandOnAFetchedColumnAndKeepsTheCount(t *testing.T) {
	for _, layout := range []rankLayout{freshLayout, migratedLayout} {
		t.Run(layout.name, func(t *testing.T) {
			setupTestDB(t)
			migrateUpTo(t, 45)
			ctx := v45Column(t, "context_length", layout, true, "openrouter", "huggingface")
			putModel(t, "M")
			putSourcedCell(t, ctx, "M", "openrouter", "128000", "2026-09-01T00:00:00Z")
			putSourcedCell(t, ctx, "M", "hand", "131072", "2026-09-02T00:00:00Z")
			before, _ := countCells()

			migrateUpTo(t, 46)

			after, _ := countCells()
			if after != before {
				t.Errorf("cells went from %d to %d, want level", before, after)
			}
			if n := countWhere(t, "source='hand'"); n != 0 {
				t.Errorf("%d hand cell(s) remain on a fetched column, want 0", n)
			}
			if n := countWhere(t, "source='research' AND value_text='131072' AND model_name='M'"); n != 1 {
				t.Errorf("the typed value came through as %d research cell(s), want 1", n)
			}
			if n := countWhere(t, "source='openrouter' AND value_text='128000'"); n != 1 {
				t.Errorf("OpenRouter's own cell was touched: %d row(s)", n)
			}
			if got := fmt.Sprint(sourcesByRank(t, ctx)); got != "[openrouter huggingface research]" {
				t.Errorf("sources by rank = %s, want [openrouter huggingface research]", got)
			}
		})
	}
}

// TestResearchLeavesHisOwnColumnsAsHand: a column only the reader states keeps
// hand as its one source and every hand cell on it, whichever layout.
func TestResearchLeavesHisOwnColumnsAsHand(t *testing.T) {
	for _, layout := range []rankLayout{freshLayout, migratedLayout} {
		t.Run(layout.name, func(t *testing.T) {
			setupTestDB(t)
			migrateUpTo(t, 45)
			notes := v45Column(t, "notes", layout, true)
			putModel(t, "M")
			putSourcedCell(t, notes, "M", "hand", "reads code well", "2026-09-02T00:00:00Z")

			migrateUpTo(t, 46)

			if got := fmt.Sprint(sourcesByRank(t, notes)); got != "[hand]" {
				t.Errorf("notes declares %s, want [hand]", got)
			}
			if n := countWhere(t, "source='hand' AND column_id=? AND value_text='reads code well'", notes); n != 1 {
				t.Errorf("the note is %d hand cell(s), want 1 untouched", n)
			}
			if n := countWhere(t, "source='research'"); n != 0 {
				t.Errorf("%d research cell(s) appeared on a column nothing automatic states", n)
			}
		})
	}
}

// TestResearchCarriesWrittenAtAcross: the re-stamped cell keeps the moment it
// was typed, because the answers panel dates it and the date is the one thing
// the move must not invent.
func TestResearchCarriesWrittenAtAcross(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 45)
	params := v45Column(t, "parameters", freshLayout, true, "huggingface")
	putModel(t, "M")
	putSourcedCell(t, params, "M", "hand", "70", "2026-07-02T10:00:00Z")
	if _, err := DB.Exec(`UPDATE cells SET value_number=70 WHERE column_id=?`, params); err != nil {
		t.Fatal(err)
	}

	migrateUpTo(t, 46)

	var writtenAt string
	var sortKey float64
	if err := DB.QueryRow(`SELECT written_at, value_number FROM cells WHERE column_id=? AND source='research'`, params).Scan(&writtenAt, &sortKey); err != nil {
		t.Fatalf("the research cell is missing: %v", err)
	}
	if writtenAt != "2026-07-02T10:00:00Z" || sortKey != 70 {
		t.Errorf("research cell carries written_at %q and sort key %v, want the hand cell's 2026-07-02T10:00:00Z and 70", writtenAt, sortKey)
	}
}

// TestResearchDeclarationRanksLastOnBothLayouts: on a column with two
// automatic sources, research lands one past the highest rank on both layouts
// and never collides with a rank in use, and a column with no fetched source
// and no hand (added) is left alone.
func TestResearchDeclarationRanksLastOnBothLayouts(t *testing.T) {
	for _, layout := range []rankLayout{freshLayout, migratedLayout} {
		t.Run(layout.name, func(t *testing.T) {
			setupTestDB(t)
			migrateUpTo(t, 45)
			moe := v45Column(t, "moe", layout, true, "huggingface", "openrouter")
			added := v45Column(t, "added", layout, false)
			if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'app',0)`, added); err != nil {
				t.Fatal(err)
			}

			migrateUpTo(t, 46)

			if got := fmt.Sprint(sourcesByRank(t, moe)); got != "[huggingface openrouter research]" {
				t.Errorf("moe declares %s, want [huggingface openrouter research]", got)
			}
			var researchRank, top int
			if err := DB.QueryRow(`SELECT rank FROM column_sources WHERE column_id=? AND source='research'`, moe).Scan(&researchRank); err != nil {
				t.Fatal(err)
			}
			if err := DB.QueryRow(`SELECT MAX(rank) FROM column_sources WHERE column_id=? AND source<>'research'`, moe).Scan(&top); err != nil {
				t.Fatal(err)
			}
			if researchRank != top+1 {
				t.Errorf("research sits at rank %d, want %d (one past the highest)", researchRank, top+1)
			}
			if got := fmt.Sprint(sourcesByRank(t, added)); got != "[app]" {
				t.Errorf("added declares %s, want [app]", got)
			}
			var kind string
			if err := DB.QueryRow(`SELECT kind FROM sources WHERE name='research'`).Scan(&kind); err != nil || kind != "research" {
				t.Errorf("the research source row: kind %q, err %v", kind, err)
			}
		})
	}
}
