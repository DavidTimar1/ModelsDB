package store

import (
	"fmt"
	"modelsdb/internal/dbcore"
)

// columnsAcceptingSource lists the bare keys of every column src may state.
func columnsAcceptingSource(src Source) ([]string, error) {
	rows, err := dbcore.DB.Query(`SELECT c.column_key FROM columns c
		JOIN column_sources cs ON cs.column_id = c.id WHERE cs.source = ?`, string(src))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// SetColumnVisible changes whether a column starts out drawn, the way a reader's
// earlier choice would have left it, so a test can check what a later seed
// leaves alone.
func SetColumnVisible(key string, visible bool) error {
	shown := 0
	if visible {
		shown = 1
	}
	res, err := dbcore.DB.Exec("UPDATE columns SET visible=? WHERE column_key=?", shown, key)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %q", ErrNoSuchColumn, key)
	}
	return nil
}

// FeedField is one row of feed_fields as a test reads it back.
type FeedField struct {
	Path     string
	Models   int
	Vanished bool
}

// ListFeedFields answers every path a refresh has recorded and whether it is
// missing from the most recent refresh that recorded anything.
func ListFeedFields() ([]FeedField, error) {
	rows, err := dbcore.DB.Query(`SELECT path, models_with_value, last_seen_run FROM feed_fields ORDER BY path ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FeedField
	var runs []int64
	var latest int64
	for rows.Next() {
		var f FeedField
		var run int64
		if err := rows.Scan(&f.Path, &f.Models, &run); err != nil {
			return nil, err
		}
		if run > latest {
			latest = run
		}
		runs = append(runs, run)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Vanished = runs[i] != latest
	}
	return out, nil
}
