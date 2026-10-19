// The record of every leaf path the OpenRouter feed has ever
// carried, and the column the refresh makes for each one nothing reads yet.
//
// A REFRESH RECORDS EVERY PATH AND MAKES A COLUMN FOR EVERY PATH NO COLUMN
// READS. The catalog package walks the raw feed record it already fetched (a
// fact this package must not itself know how to read - see
// catalog/feedfields.go), decides which paths need a column, and hands this
// file the result to persist. What this file owns is the table itself, the
// new/vanished comparison a refresh reports, the one column a path becomes,
// and the seal every such column carries.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"sort"
	"strings"
)

// FieldObservation is what one refresh saw for one leaf path: the JSON kind it
// carried, how many models stated a non-null value for it this refresh, and one
// sample value to show beside it. Kind is empty when every model's value for
// the path was null this refresh - RecordFeedFields then keeps whatever kind
// was last recorded, or falls back to text for a path never seen non-null at
// all.
type FieldObservation struct {
	Kind   string
	Models int
	Sample string
}

// RecordFeedFields persists one refresh's observation of every leaf path the
// feed carried this time, and answers which paths are new (never recorded
// before this refresh) and which vanished (recorded as of the previous
// refresh, and absent from this one).
//
// THE VERY FIRST REFRESH EVER REPORTS NEITHER. A database with no feed_fields
// rows yet has nothing to compare against, so calling everything "new" on
// that first run would be noise, not news.
func RecordFeedFields(observed map[string]FieldObservation) (newPaths, vanishedPaths []string, err error) {
	tx, err := dbcore.DB.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	firstEverRun, err := feedFieldsTableEmpty(tx)
	if err != nil {
		return nil, nil, err
	}
	existing, err := existingFeedFieldStamps(tx)
	if err != nil {
		return nil, nil, err
	}
	previousRun := maxRun(existing)
	thisRun := previousRun + 1
	now := dbcore.NowStamp()

	paths := make([]string, 0, len(observed))
	for p := range observed {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	seenNow := map[string]bool{}
	for _, path := range paths {
		seenNow[path] = true
		obs := observed[path]
		kind := obs.Kind
		if held, ok := existing[path]; kind == "" && ok {
			kind = held.kind
		} else if kind == "" {
			kind = ValueTypeText
		}
		firstSeen := now
		if row, ok := existing[path]; ok {
			firstSeen = row.firstSeen
		}
		if _, err := tx.Exec(`INSERT INTO feed_fields (path, kind, models_with_value, sample_value, first_seen, last_seen, last_seen_run)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT(path) DO UPDATE SET
			  kind=excluded.kind, models_with_value=excluded.models_with_value,
			  sample_value=excluded.sample_value, last_seen=excluded.last_seen, last_seen_run=excluded.last_seen_run`,
			path, kind, obs.Models, obs.Sample, firstSeen, now, thisRun); err != nil {
			return nil, nil, fmt.Errorf("record the feed field %s: %w", path, err)
		}
		if !firstEverRun {
			if _, held := existing[path]; !held {
				newPaths = append(newPaths, path)
			}
		}
	}

	if !firstEverRun {
		for path, row := range existing {
			if row.lastSeenRun == previousRun && !seenNow[path] {
				vanishedPaths = append(vanishedPaths, path)
			}
		}
		sort.Strings(vanishedPaths)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return newPaths, vanishedPaths, nil
}

// feedFieldStamp is one existing row's kind, timestamps and run number, read
// before a refresh overwrites them, so RecordFeedFields can tell a path that
// is new from one merely being restated.
type feedFieldStamp struct {
	kind, firstSeen, lastSeen string
	lastSeenRun               int64
}

func feedFieldsTableEmpty(tx *sql.Tx) (bool, error) {
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM feed_fields").Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}

func existingFeedFieldStamps(tx *sql.Tx) (map[string]feedFieldStamp, error) {
	rows, err := tx.Query("SELECT path, kind, first_seen, last_seen, last_seen_run FROM feed_fields")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]feedFieldStamp{}
	for rows.Next() {
		var path string
		var s feedFieldStamp
		if err := rows.Scan(&path, &s.kind, &s.firstSeen, &s.lastSeen, &s.lastSeenRun); err != nil {
			return nil, err
		}
		out[path] = s
	}
	return out, rows.Err()
}

// maxRun finds the highest last_seen_run among rows recorded before this
// refresh - the run number every path present in the PREVIOUS refresh was
// stamped with, since one refresh stamps everything it saw with the same
// number. A run number, not a timestamp, is what tells two refreshes apart
// even when they land in the same wall-clock second.
func maxRun(rows map[string]feedFieldStamp) int64 {
	var highest int64
	for _, row := range rows {
		if row.lastSeenRun > highest {
			highest = row.lastSeenRun
		}
	}
	return highest
}

// feedColumnSources is what every column made from a feed path declares: the
// refresh fills it, and research may fill a gap until the feed does - exactly
// what every shipped OpenRouter column declares.
func feedColumnSources() []Source { return []Source{SourceOpenRouter, SourceResearch} }

// CreateFeedFieldColumn makes the column for one recorded feed path: keyed by
// the whole path, hidden, SEALED, and sourced as feedColumnSources says. The
// path is tied to the column through feed_fields.column_id, which is what the
// fill and the column list's feed_paths run on.
//
// SEALED, LIKE A COLUMN THE APP SHIPS: it can be relabelled, moved and hidden,
// never deleted or re-typed. The next refresh would make it again, so a delete
// would be a lie.
//
// A PATH NEVER SEEN IN A REFRESH IS REFUSED: only a refresh records the paths
// the feed carried (see catalog/feedfields.go), so there is nothing to tie a
// column to until one has run. A path already tied to a column answers that
// column unchanged, whatever key it was given: a path is never made twice.
func CreateFeedFieldColumn(path, valueType string) (Column, error) {
	if valueType == "" {
		valueType = ValueTypeText
	}
	if valueType == ValueTypeChoice || !ValidValueType(valueType) {
		return Column{}, fmt.Errorf("%q is not a kind of value a column made from the feed can hold: %s",
			valueType, quotedList([]string{ValueTypeText, ValueTypeNumber, ValueTypeFlag, ValueTypeList}))
	}

	var existingColumnID sql.NullInt64
	err := dbcore.DB.QueryRow("SELECT column_id FROM feed_fields WHERE path=?", path).Scan(&existingColumnID)
	if errors.Is(err, sql.ErrNoRows) {
		return Column{}, fmt.Errorf("%q has never been seen in a refresh, so there is nothing to make a column from yet", path)
	}
	if err != nil {
		return Column{}, err
	}
	if existingColumnID.Valid {
		return columnByID(existingColumnID.Int64)
	}

	// The shipped columns are what a key or a label collides with, so they have
	// to be there before either is checked.
	if err := ensureShippedColumns(); err != nil {
		return Column{}, err
	}
	key := columnKeyForFeedPath(path)
	if err := checkColumnKeyNotReserved(key); err != nil {
		return Column{}, err
	}
	if held, err := columnExists(key); err != nil {
		return Column{}, err
	} else if held {
		return Column{}, fmt.Errorf("%s already exists as a column, so no second one was made for %q", key, path)
	}
	label, err := feedColumnLabel(key, path)
	if err != nil {
		return Column{}, err
	}
	spec := ColumnSpec{
		Key: key, Label: label, ValueType: valueType, About: feedFieldAbout(key, path),
		Position: ownColumnPosition, Hidden: true, Sealed: true,
	}

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return Column{}, err
	}
	defer tx.Rollback()

	id, err := insertColumn(tx, spec, nil)
	if err != nil {
		return Column{}, err
	}
	if err := declareColumnSources(tx, id, feedColumnSources()); err != nil {
		return Column{}, err
	}
	if _, err := tx.Exec("UPDATE feed_fields SET column_id=? WHERE path=?", id, path); err != nil {
		return Column{}, err
	}
	if err := tx.Commit(); err != nil {
		return Column{}, err
	}
	return ColumnByKey(key)
}

// columnByID reads one column by its row id, which is how feed_fields names
// the column a path is tied to.
func columnByID(id int64) (Column, error) {
	var key string
	if err := dbcore.DB.QueryRow("SELECT column_key FROM columns WHERE id=?", id).Scan(&key); err != nil {
		return Column{}, fmt.Errorf("read the column with id %d: %w", id, err)
	}
	return ColumnByKey(key)
}

// feedColumnLabel words a column made from the feed for the top of the table:
// the key's words with the first letter capitalised ("Top provider context
// length"). A label another column already carries would fail the insert on
// the unique index over labels, so it gains the path in brackets instead; the
// reader renames what they want.
func feedColumnLabel(key, path string) (string, error) {
	label := labelFromKey(key)
	_, held, err := columnByLabel(label)
	if err != nil {
		return "", err
	}
	if held {
		label = label + " (" + path + ")"
	}
	return label, nil
}

// SealFeedFieldColumns seals every column tied to a feed path that is not
// sealed yet, and answers how many it sealed. A column tied to a feed path that
// was not made by a refresh is unsealed; a refresh would make it again, so it is
// sealed the same as one the refresh made.
func SealFeedFieldColumns() (int64, error) {
	res, err := dbcore.DB.Exec(`UPDATE columns SET sealed=1 WHERE sealed=0
		AND id IN (SELECT column_id FROM feed_fields WHERE column_id IS NOT NULL)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ImportedFeedFieldColumns answers every column tied to a feed path, keyed by
// its column key, with the path that fills it.
func ImportedFeedFieldColumns() (map[string]string, error) {
	rows, err := dbcore.DB.Query(`SELECT c.column_key, f.path FROM feed_fields f JOIN columns c ON c.id = f.column_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, path string
		if err := rows.Scan(&key, &path); err != nil {
			return nil, err
		}
		out[key] = path
	}
	return out, rows.Err()
}

// columnKeyForFeedPath derives a column key from a dotted, possibly "[]"-
// bearing feed path: the WHOLE path, lowercased, with "[]" removed and every
// dot an underscore, so "top_provider.context_length" is
// top_provider_context_length and "benchmarks.design_arena[].elo" is
// benchmarks_design_arena_elo. The last segment alone would make keys like
// rank, elo and name, which say nothing about where they came from and
// collide with a reader's own column. columnStem does the lowercasing and the
// dot, and turns any character a key cannot carry into the same underscore.
func columnKeyForFeedPath(path string) string {
	return columnStem(strings.ReplaceAll(path, "[]", ""))
}
