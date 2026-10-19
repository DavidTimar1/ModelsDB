// How many models have answered each column.
//
// The browser needs this figure for every column at once. One GROUP BY over the
// cells table gives it without reading the whole catalog.

package store

import "modelsdb/internal/dbcore"

// AnswerCounts reports, per column key, how many models hold a value in that
// column, over the WHOLE catalog - hidden rows and variations included, exactly
// as the value lists are computed. Every column is present, including one no
// model has answered, whose count is zero.
//
// A cell is never stored empty (writeOneCell deletes the row instead), so a
// stored cell IS an answer and no value test is needed here.
func AnswerCounts() (map[string]int, error) {
	rows, err := dbcore.DB.Query(`SELECT c.column_key, COUNT(v.model_name)
		FROM columns c LEFT JOIN cells v ON v.column_id = c.id
		GROUP BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		out[key] = n
	}
	return out, rows.Err()
}
