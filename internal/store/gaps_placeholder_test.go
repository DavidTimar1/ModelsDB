package store

import "testing"

// The placeholder in the brief's example follows the column's own kind of value,
// not the sentence that describes it.
func TestAGapsPlaceholderFollowsTheKindOfItsColumn(t *testing.T) {
	seedInto(t)
	for key, want := range map[string]string{
		"context_length":   "<the number you found, in quotes>",
		"supports_tools":   `<"yes" or "no", in quotes>`,
		"input_modalities": `<the list you found as quoted text, e.g. "[\"text\",\"image\"]">`,
		"model_type":       "<one of the words listed above, in quotes>",
	} {
		column, err := ColumnByKey(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if got := placeholderOf(column); got != want {
			t.Errorf("placeholder of %s = %q, want %q", key, got, want)
		}
	}
}
