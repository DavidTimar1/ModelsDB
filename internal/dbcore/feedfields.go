package dbcore

import "database/sql"

// This file is migration v45: adds feed_fields, the record of every leaf path
// the OpenRouter feed has ever carried. Before this migration the feed was
// read once per refresh and thrown away except for the fixed set of facts
// catalog.OpenRouterFacts already knew to look for; a field the feed added or
// dropped was invisible until someone happened to read the feed by hand. This
// table is what a refresh writes every leaf path into, and what ties a path
// to the column the refresh makes for it.
//
// column_id is NULL for a path no column reads, and is set the moment the
// refresh makes a column for it (store.CreateFeedFieldColumn). Such a column
// is sealed and cannot be deleted; ON DELETE SET NULL is what keeps the
// path's own history (first seen, last seen) on record should the column row
// ever go, so a later refresh does not try to write a stale column id.
//
// last_seen_run is a counter, not a timestamp: last_seen only has second
// precision, so two refreshes in the same second (which a real refresh never
// does, but a fast test can) would tie on last_seen and make "vanished" (and
// which run is the latest) ambiguous. Every path a refresh touches gets that
// refresh's run number, one higher than the previous highest.
func theFeedFieldsTableIsAdded(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE feed_fields (
		path              TEXT    PRIMARY KEY NOT NULL,
		kind              TEXT    NOT NULL DEFAULT 'text' CHECK (kind IN ('text','number','flag','list')),
		models_with_value INTEGER NOT NULL DEFAULT 0,
		sample_value      TEXT    NOT NULL DEFAULT '',
		first_seen        TEXT    NOT NULL DEFAULT '',
		last_seen         TEXT    NOT NULL DEFAULT '',
		last_seen_run     INTEGER NOT NULL DEFAULT 0,
		column_id         INTEGER REFERENCES columns(id) ON DELETE SET NULL
	)`); err != nil {
		return err
	}
	_, err := tx.Exec(`CREATE INDEX idx_feed_fields_column ON feed_fields(column_id) WHERE column_id IS NOT NULL`)
	return err
}
