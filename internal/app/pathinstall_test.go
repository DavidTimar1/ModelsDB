package app

import (
	"strings"
	"testing"
)

// TestRcBlockRoundTrip verifies the managed shell-rc block is added idempotently
// and removed cleanly, never disturbing the user's own lines.
func TestRcBlockRoundTrip(t *testing.T) {
	user := "# my rc\nexport EDITOR=vim\nalias ll='ls -l'\n"

	added := rcBlockAdd(user, "/home/user/apps/modelsdb")
	if !rcBlockPresent(added) {
		t.Fatal("block should be present after add")
	}
	if !strings.Contains(added, `export PATH="/home/user/apps/modelsdb:$PATH"`) {
		t.Errorf("PATH line missing:\n%s", added)
	}
	if !strings.HasPrefix(added, user) {
		t.Errorf("user content must be preserved verbatim at the top:\n%s", added)
	}

	// Adding again must not stack a second block.
	twice := rcBlockAdd(added, "/home/user/apps/modelsdb")
	if n := strings.Count(twice, pathBlockBegin); n != 1 {
		t.Errorf("expected exactly 1 managed block, got %d", n)
	}

	// Re-adding with a new dir updates in place.
	moved := rcBlockAdd(added, "/opt/modelsdb")
	if strings.Contains(moved, "/home/user/apps/modelsdb") || !strings.Contains(moved, "/opt/modelsdb") {
		t.Errorf("re-add should replace the dir:\n%s", moved)
	}

	// Removal restores the original user content.
	removed := rcBlockRemove(added)
	if removed != user {
		t.Errorf("remove should restore original.\n got: %q\nwant: %q", removed, user)
	}
	if rcBlockPresent(removed) {
		t.Error("block should be absent after remove")
	}
	// Removing when absent is a no-op.
	if got := rcBlockRemove(user); got != user {
		t.Errorf("remove on absent block changed content: %q", got)
	}
}

func TestRcBlockEmptyFile(t *testing.T) {
	added := rcBlockAdd("", "/opt/modelsdb")
	if !rcBlockPresent(added) {
		t.Fatal("block missing")
	}
	if got := rcBlockRemove(added); got != "" {
		t.Errorf("removing the only block should yield empty, got %q", got)
	}
}

func TestPathList(t *testing.T) {
	const sep = ";"
	list := `C:\Windows;C:\Windows\System32`

	if pathListContains(list, sep, `C:\Apps\modelsdb`) {
		t.Error("dir should not be present yet")
	}
	// Case-insensitive + trailing-slash-insensitive match.
	if !pathListContains(list, sep, `c:\windows\`) {
		t.Error("expected case/slash-insensitive match")
	}

	added, changed := pathListAdd(list, sep, `C:\Apps\modelsdb`)
	if !changed || !pathListContains(added, sep, `C:\Apps\modelsdb`) {
		t.Errorf("add failed: %q", added)
	}
	if _, changed := pathListAdd(added, sep, `C:\Apps\modelsdb`); changed {
		t.Error("adding an existing dir must not change the list")
	}

	removed, changed := pathListRemove(added, sep, `C:\Apps\modelsdb`)
	if !changed || pathListContains(removed, sep, `C:\Apps\modelsdb`) {
		t.Errorf("remove failed: %q", removed)
	}
	if removed != list {
		t.Errorf("remove should restore original list.\n got: %q\nwant: %q", removed, list)
	}
	if _, changed := pathListRemove(list, sep, `C:\Nope`); changed {
		t.Error("removing an absent dir must not change the list")
	}
}

func TestPathListAddEmpty(t *testing.T) {
	got, changed := pathListAdd("", ";", `C:\Apps\modelsdb`)
	if !changed || got != `C:\Apps\modelsdb` {
		t.Errorf("add to empty list = %q", got)
	}
}
