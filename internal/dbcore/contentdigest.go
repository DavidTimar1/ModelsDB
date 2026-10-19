// A digest of what the database HOLDS, so the daily backup can tell a changed
// answer from a moved filter.
//
// The reader's view is stored in this database, so the file changes when
// somebody sorts a column, and a measure of the file alone (its size and
// modification time) would collect a backup a day of a catalog nobody touched.
//
// app_settings IS THE ONE TABLE LEFT OUT. Every other table is read, and the
// list comes from sqlite_master rather than from a list written here, so a table
// added later is covered without anybody remembering to add it.

package dbcore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// contentDigestSkip names the tables a digest ignores: the reader's view is not
// their data, and SQLite's own bookkeeping tables are not either.
var contentDigestSkip = map[string]bool{"app_settings": true}

// ContentDigest returns a hash of everything the database holds except the
// reader's stored view.
//
// AN ERROR MEANS "CHANGED", and every caller must treat it that way: an extra
// backup file costs disk, a missed one costs the data. It returns the error so
// the caller can log it, never so the caller can decide the data is unchanged.
func ContentDigest() (string, error) {
	tables, err := digestTables()
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	for _, table := range tables {
		fmt.Fprintf(sum, "\x00table:%s\x00", table)
		if err := digestTable(sum, table); err != nil {
			return "", fmt.Errorf("digest %s: %w", table, err)
		}
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func digestTables() ([]string, error) {
	rows, err := DB.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if contentDigestSkip[name] {
			continue
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// digestTable hashes one table's rows.
//
// The rows are ordered by their own columns rather than by rowid, so that a
// VACUUM, a restore, or a delete-and-reinsert that lands the same values in a
// different physical order still digests the same. A WITHOUT ROWID table has no
// rowid to order by at all, which is the other reason.
func digestTable(sum io.Writer, table string) error {
	cols, err := digestColumns(table)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	quoted := make([]string, 0, len(cols))
	for _, c := range cols {
		quoted = append(quoted, `"`+strings.ReplaceAll(c, `"`, `""`)+`"`)
	}
	list := strings.Join(quoted, ",")
	// The table and column names come from sqlite_master, never from a request,
	// and are quoted above; SQLite takes no placeholder in either position.
	rows, err := DB.Query(`SELECT ` + list + ` FROM "` + strings.ReplaceAll(table, `"`, `""`) + `" ORDER BY ` + list)
	if err != nil {
		return err
	}
	defer rows.Close()

	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		for _, v := range vals {
			switch t := v.(type) {
			case nil:
				fmt.Fprint(sum, "\x00null\x00")
			case []byte:
				sum.Write(t) //nolint:errcheck // a hash never fails a write
				fmt.Fprint(sum, "\x00")
			default:
				fmt.Fprintf(sum, "%v\x00", t)
			}
		}
		fmt.Fprint(sum, "\x00row\x00")
	}
	return rows.Err()
}

func digestColumns(table string) ([]string, error) {
	rows, err := DB.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
