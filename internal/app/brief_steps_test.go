package app

import (
	"modelsdb/internal/store"
	"strings"
	"testing"
)

// The numbered steps of the brief run without a gap: the size warning is step 4
// when it is asked for and absent otherwise, and the writing step follows it.
func TestTheBriefsStepsAreNumberedWithoutAGap(t *testing.T) {
	addresses := []string{"http://127.0.0.1:8122"}

	without := briefText("M", "", store.RowBrief{Gaps: []store.Gap{{Field: "notes_x", Label: "X", Accepts: "text in quotes"}}}, addresses, "[]")
	if !strings.Contains(without, "\n4. Write only the fields") || strings.Contains(without, "\n5. ") {
		t.Errorf("without a size gap the writing step must be 4:\n%s", without)
	}

	with := briefText("M", "", store.RowBrief{Gaps: []store.Gap{{Field: "parameters", Label: "P", Accepts: "text in quotes"}}}, addresses, "[]")
	if !strings.Contains(with, "\n4. A repository that publishes") || !strings.Contains(with, "\n5. Write only the fields") {
		t.Errorf("with a size gap the size warning is 4 and writing is 5:\n%s", with)
	}
}
