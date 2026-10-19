package catalog

import (
	"fmt"
	"testing"
)

func TestOneRefreshMakesAtMostAFixedNumberOfFeedColumns(t *testing.T) {
	few := []string{"a", "b"}
	if got := limitNewFeedColumns(few); len(got) != 2 {
		t.Errorf("a short list was cut to %d", len(got))
	}
	many := make([]string, 0, maxNewFeedColumns+30)
	for i := 0; i < maxNewFeedColumns+30; i++ {
		many = append(many, fmt.Sprintf("path_%d", i))
	}
	got := limitNewFeedColumns(many)
	if len(got) != maxNewFeedColumns || got[0] != "path_0" {
		t.Errorf("got %d paths starting %q, want %d starting path_0", len(got), got[0], maxNewFeedColumns)
	}
}
