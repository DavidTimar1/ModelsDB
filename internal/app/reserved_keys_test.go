package app

import (
	"modelsdb/internal/store"
	"testing"
)

// The words the query reads before it looks for a column are exactly the words
// the store refuses as column keys.
func TestEveryWordTheQueryReadsFirstIsAReservedColumnKey(t *testing.T) {
	words := append([]string{paramName, paramIncludeHidden, paramHideFree, paramLinks, paramFields}, identityFields...)
	for _, word := range words {
		if !store.IsReservedColumnKey(word) {
			t.Errorf("%q is read by the query as something other than a column but a column may be called that", word)
		}
	}
}
