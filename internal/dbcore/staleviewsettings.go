package dbcore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// This file is migration v43: rewrites the reader's own saved view settings,
// stale since the namespace pivot stripped every column's dotted prefix
// (pivotcontract.go, v41). app_settings holds one JSON blob per view per
// setting, keyed "view.<id>.<name>" - col_hidden, col_known and col_order are
// each a flat list of column keys, and order is a list of [key, direction]
// pairs (app.js's readStoredViews and its VIEW_TABLE_KEYS). None of those
// blobs were touched by any pivot migration, so every entry that named a real
// column still carries the old "openrouter.context_length" shape, which
// matches nothing in the now-bare columns table - applyColOrder and
// resolveHiddenTargets (app.js) already tolerate this by silently not
// matching, so a saved view quietly stopped hiding or ordering what it used
// to, rather than erroring.
//
// WHAT IT TOUCHES.
//   - app_settings: every row whose key matches view.<id>.(col_hidden|
//     col_known|col_order|order) has its JSON value rewritten in place. No
//     other row, and no other table.
//
// WHAT IS DELIBERATELY LEFT ALONE. Two virtual entries, "_name" and
// "_variations" (app.js's NAME_TARGET/VAR_TARGET), can appear in col_order
// and order but never correspond to a row in columns - they never carried a
// namespace prefix, so they are never touched here. Only an entry that
// contains a dot is treated as an old dotted column key: it is stripped of
// its namespace and re-validated against columns.column_key, and dropped if
// nothing there matches. This is narrower than "look up every entry": doing
// that instead would need special-casing the two virtual targets by name
// here, which ties this migration to app.js's own constants; scoping the
// transform to dotted entries needs no such coupling, because every real
// pre-pivot column key was guaranteed dotted by the schema's own CHECK
// (schemaSQL, db.go) and no legitimate bare entry is anything but an
// already-current column key or one of the two virtual targets.
//
// A blob that is absent, empty, or not valid JSON is left exactly as it was:
// this migration only ever improves a value it can fully parse, never trades
// a stale-but-readable setting for a lost one.
func theStaleViewSettingsAreRewritten(tx *sql.Tx) error {
	held, err := tableExists(tx, "app_settings")
	if err != nil {
		return err
	}
	if !held {
		return nil // no settings table yet, nothing to rewrite
	}

	validColumns, err := currentColumnKeys(tx)
	if err != nil {
		return err
	}

	rows, err := tx.Query(`SELECT key, value FROM app_settings WHERE key LIKE 'view.%'`)
	if err != nil {
		return fmt.Errorf("read the stored view settings: %w", err)
	}
	type kv struct{ key, value string }
	var candidates []kv
	for rows.Next() {
		var r kv
		if err := rows.Scan(&r.key, &r.value); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, c := range candidates {
		field := viewSettingFieldPattern.FindStringSubmatch(c.key)
		if field == nil {
			continue // not one of the four fields this migration rewrites
		}
		var rewritten string
		var changed bool
		var err error
		if field[1] == "order" {
			rewritten, changed, err = rewriteOrderPairs(c.value, validColumns)
		} else {
			rewritten, changed, err = rewriteFlatKeyList(c.value, validColumns)
		}
		if err != nil {
			return fmt.Errorf("rewrite %s: %w", c.key, err)
		}
		if !changed {
			continue
		}
		if _, err := tx.Exec(`UPDATE app_settings SET value = ? WHERE key = ?`, rewritten, c.key); err != nil {
			return fmt.Errorf("store the rewritten %s: %w", c.key, err)
		}
		log.Printf("Migration v43: rewrote stale column keys in %s", c.key)
	}
	return nil
}

// viewSettingFieldPattern matches the four view.<id>.<field> settings that
// name columns, capturing the field name. <id> is not anchored to "v1": a
// database can hold more than one saved view.
var viewSettingFieldPattern = regexp.MustCompile(`^view\.[^.]+\.(col_hidden|col_known|col_order|order)$`)

// currentColumnKeys is the post-pivot set a stripped dotted key is validated
// against: an entry that strips down to nothing here named a column that no
// longer exists (renamed, merged away, or never real), and is dropped.
func currentColumnKeys(tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.Query(`SELECT column_key FROM columns`)
	if err != nil {
		return nil, fmt.Errorf("read the current column keys: %w", err)
	}
	defer rows.Close()
	keys := make(map[string]bool)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys[k] = true
	}
	return keys, rows.Err()
}

// stripNamespace removes everything up to and including the first dot. An
// entry with no dot is returned unchanged - callers only pass this a dotted
// entry, but it stays a total function rather than one that can panic on a
// shape it was not expecting.
func stripNamespace(key string) string {
	if i := strings.IndexByte(key, '.'); i >= 0 {
		return key[i+1:]
	}
	return key
}

// rewriteFlatKeyList transforms col_hidden, col_known and col_order: each is
// a flat JSON array of strings. A malformed blob (not a JSON string array) is
// reported as unchanged rather than an error, so the caller leaves it as-is.
func rewriteFlatKeyList(raw string, validColumns map[string]bool) (string, bool, error) {
	var entries []string
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return raw, false, nil // not this shape - leave it alone, not an error
	}

	out := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	changed := false
	for _, entry := range entries {
		next := entry
		if strings.Contains(entry, ".") {
			bare := stripNamespace(entry)
			if !validColumns[bare] {
				changed = true // dropped: no current column matches
				continue
			}
			next = bare
			changed = true
		}
		if seen[next] {
			changed = true // two old keys collapsed onto one bare key - keep the first
			continue
		}
		seen[next] = true
		out = append(out, next)
	}
	if !changed {
		return raw, false, nil
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw, false, err
	}
	return string(encoded), true, nil
}

// rewriteOrderPairs transforms the "order" setting: a JSON array of
// [columnKeyOrTarget, direction] pairs, structurally distinct from the flat
// lists above because the direction string must travel with its key. A
// malformed blob, or a pair that is not itself a two-element array, is
// reported as unchanged rather than an error.
func rewriteOrderPairs(raw string, validColumns map[string]bool) (string, bool, error) {
	var pairs [][]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &pairs); err != nil {
		return raw, false, nil
	}

	out := make([][]json.RawMessage, 0, len(pairs))
	seen := make(map[string]bool, len(pairs))
	changed := false
	for _, pair := range pairs {
		if len(pair) != 2 {
			return raw, false, nil // not the [key, direction] shape - leave the whole blob alone
		}
		var key string
		if err := json.Unmarshal(pair[0], &key); err != nil {
			return raw, false, nil // key element is not a string - leave the whole blob alone
		}
		next := key
		if strings.Contains(key, ".") {
			bare := stripNamespace(key)
			if !validColumns[bare] {
				changed = true // dropped: no current column matches
				continue
			}
			next = bare
			changed = true
		}
		if seen[next] {
			changed = true // two old keys collapsed onto one bare key - keep the first
			continue
		}
		seen[next] = true
		if next != key {
			encodedKey, err := json.Marshal(next)
			if err != nil {
				return raw, false, err
			}
			out = append(out, []json.RawMessage{encodedKey, pair[1]})
		} else {
			out = append(out, pair)
		}
	}
	if !changed {
		return raw, false, nil
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw, false, err
	}
	return string(encoded), true, nil
}
