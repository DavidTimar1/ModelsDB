package dbcore

import "testing"

// TestCountTopLevelModelsLeavesOutOnlyAVariationWithAParentHere: a variation is
// drawn under its parent and so is not a row of its own, but a variation whose
// parent is gone, and a variation of a variation, are drawn as ordinary rows.
func TestCountTopLevelModelsLeavesOutOnlyAVariationWithAParentHere(t *testing.T) {
	setupTestDB(t)
	for _, row := range [][2]string{
		{"Parent", ""}, {"Plain", ""}, {"Build", "Parent"}, {"Orphan", "Gone"}, {"Nested", "Build"},
	} {
		if _, err := DB.Exec("INSERT INTO models (name, variant_of) VALUES (?,?)", row[0], row[1]); err != nil {
			t.Fatal(err)
		}
	}
	n, err := CountTopLevelModels()
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("CountTopLevelModels = %d, want 4 (every row but Build)", n)
	}
}
