package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

// A symlink planted at a predictable temp name must never be followed.
func TestStageNeverFollowsAPlantedTempSymlink(t *testing.T) {
	priv, pubB64 := newKey(t)
	r := signedRelease(t, priv, "1.3.0", []byte("new binary"))
	s, exe := stagerFor(t, r, pubB64, "1.2.0")

	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, planted := range []string{".modelsdb-update.tmp", ".modelsdb-update-0", ".modelsdb-update-1"} {
		if err := os.Symlink(victim, filepath.Join(filepath.Dir(exe), planted)); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
	}
	if staged, err := s.run("1.3.0", nil); err != nil || !staged {
		t.Fatalf("run = (%v, %v)", staged, err)
	}
	if got, _ := os.ReadFile(victim); string(got) != "precious" {
		t.Fatalf("a planted symlink was followed; victim now holds %q", got)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new binary" {
		t.Fatalf("executable holds %q", got)
	}
	fi, err := os.Stat(exe)
	if err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("staged binary is not executable: %v %v", fi, err)
	}
}
