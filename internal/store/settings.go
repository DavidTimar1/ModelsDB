// The reader's view and the app's preferences, one row per
// preference in app_settings.
//
// THESE ROWS ARE NOT CELLS. A cell is a fact about a model, stated by a source
// that owns its column. A preference is a fact about this install's view - which
// columns are drawn, in what order, at what page size - and it belongs to no
// model, so nothing here is ever written into cells.
//
// One row per preference, not one blob in one row: a blob is a settings file in
// a different container, and one unreadable byte in it costs every preference at
// once. A damaged row here costs its own preference and nothing else.
//
// THE SERVER DOES NOT KNOW THE KEY LIST. Whatever keys the browser sends are the
// keys that are stored, and none of them is validated, so a new preference in
// the page needs no change here. The value is stored as the JSON the browser
// sent, so true, 25, ["a","b"] and a nested object all come back with their
// types intact.
//
// A WRITE NAMES WHAT IT STORES AND WHAT IT REMOVES. WriteSettings takes the keys
// to store and the keys to remove, and a key in neither half is left exactly as
// it was. That is what lets two pages, holding different ideas of which
// preferences exist, write beside each other: neither can remove a preference it
// has never heard of.
//
// ONE NAMESPACE IS RESERVED. A key beginning "server." belongs to the server: a
// write naming one, in either half, is refused before anything is stored.
// Without that rule a filter change would destroy anything the server had
// recorded here.

package store

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"strings"
)

// ServerSettingPrefix marks a preference row the SERVER owns. The browser may
// read such a row and may not write one.
const ServerSettingPrefix = "server."

// SettingsResetNotice is the row the migration that moved the reader's view into
// the database writes on a database that already held models, saying that the
// view and the backup schedule did not come across from the settings file. The
// page shows one bar for it and POST /api/settings/ack-notice deletes it.
//
// This exact string is repeated in internal/dbcore/migrate.go, which writes the
// row: dbcore sits below store in the import DAG (paths -> shared -> dbcore ->
// store), so it cannot read this constant. Change both together.
const SettingsResetNotice = ServerSettingPrefix + "notice.settings_reset"

// ErrReservedSetting is returned for a write naming a key in the server's own
// namespace.
var ErrReservedSetting = fmt.Errorf("a settings key beginning %q belongs to the server", ServerSettingPrefix)

// ErrEmptySettingKey is returned for a write naming the empty key. It is the
// primary key of app_settings, so a row stored under it is one nothing can ever
// name again.
var ErrEmptySettingKey = fmt.Errorf("a settings key cannot be empty")

// IsReservedSettingKey reports whether key belongs to the server's namespace.
func IsReservedSettingKey(key string) bool {
	return strings.HasPrefix(key, ServerSettingPrefix)
}

// ListSettings returns every stored preference, the server's own rows included,
// with each value as the JSON it was stored as.
//
// A row whose value does not parse is DROPPED and logged rather than failing the
// read: one damaged preference must not cost the reader every other one, and the
// page falls back to that preference's default.
func ListSettings() (map[string]json.RawMessage, error) {
	rows, err := dbcore.DB.Query("SELECT key, value FROM app_settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]json.RawMessage{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if !json.Valid([]byte(value)) {
			log.Printf("Warning: dropping setting %q: its stored value is not valid JSON", key)
			continue
		}
		out[key] = json.RawMessage(value)
	}
	return out, rows.Err()
}

// ReadSetting returns one stored preference and whether it is there. A row whose
// value does not parse reads as absent.
func ReadSetting(key string) (json.RawMessage, bool) {
	var value string
	if err := dbcore.DB.QueryRow("SELECT value FROM app_settings WHERE key=?", key).Scan(&value); err != nil {
		return nil, false
	}
	if !json.Valid([]byte(value)) {
		return nil, false
	}
	return json.RawMessage(value), true
}

// ReadSettingBool returns one stored preference as a boolean, and false for a
// key that is absent or holds anything else.
func ReadSettingBool(key string) bool {
	raw, ok := ReadSetting(key)
	if !ok {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) != nil {
		return false
	}
	return b
}

// ReadSettingBoolOr is ReadSettingBool for a preference that is ON until it is
// stored as off: a key that is absent, or holds anything but a boolean, reads as
// fallback.
func ReadSettingBoolOr(key string, fallback bool) bool {
	raw, ok := ReadSetting(key)
	if !ok {
		return fallback
	}
	var b bool
	if json.Unmarshal(raw, &b) != nil {
		return fallback
	}
	return b
}

// DeleteSetting removes one stored preference, the server's own rows included.
// A key that is not there is not an error.
func DeleteSetting(key string) error {
	_, err := dbcore.DB.Exec("DELETE FROM app_settings WHERE key=?", key)
	return err
}

// WriteSettings stores every key of set and removes every key of del, in ONE
// transaction, and leaves every other stored preference exactly as it was.
//
// A key nobody names is a key nobody touches. A page that loaded before a
// preference existed does not carry it, and must not be able to remove it by not
// carrying it: it names what it means to remove instead. Naming the same key in
// both halves is the caller's bug and is refused by the route, so the order the
// two halves run in here is not a rule anything relies on.
//
// Both halves are checked before a transaction is opened, and the store does its
// own checking rather than trusting the route to have done it.
func WriteSettings(set map[string]json.RawMessage, del []string) error {
	for key := range set {
		if err := checkWritableSettingKey(key); err != nil {
			return err
		}
	}
	for _, key := range del {
		if err := checkWritableSettingKey(key); err != nil {
			return err
		}
	}
	if len(set) == 0 && len(del) == 0 {
		return nil
	}

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(set) > 0 {
		upsert, err := tx.Prepare("INSERT INTO app_settings (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value")
		if err != nil {
			return err
		}
		defer upsert.Close()
		for key, value := range set {
			if _, err := upsert.Exec(key, string(value)); err != nil {
				return fmt.Errorf("store the preference %q: %w", key, err)
			}
		}
	}
	if len(del) > 0 {
		remove, err := tx.Prepare("DELETE FROM app_settings WHERE key=?")
		if err != nil {
			return err
		}
		defer remove.Close()
		for _, key := range del {
			if _, err := remove.Exec(key); err != nil {
				return fmt.Errorf("remove the preference %q: %w", key, err)
			}
		}
	}
	return tx.Commit()
}

// checkWritableSettingKey answers whether the browser may name this key.
func checkWritableSettingKey(key string) error {
	if key == "" {
		return ErrEmptySettingKey
	}
	if IsReservedSettingKey(key) {
		return fmt.Errorf("%w: %q", ErrReservedSetting, key)
	}
	return nil
}
