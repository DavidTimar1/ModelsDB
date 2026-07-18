//go:build windows

package selfupdate

import "os"

// apply swaps in the new binary on Windows, where a running .exe cannot be
// overwritten in place. Windows does allow renaming a running executable, so the
// current file is moved aside to "<self>.old" and the verified temp file is
// renamed into its place. The leftover .old is removed on the next startup by
// CleanupOnStartup. If the second rename fails, the original is moved back so the
// app still has a working binary.
func apply(self, temp string) error {
	old := self + ".old"
	os.Remove(old) // clear any stale leftover first
	if err := os.Rename(self, old); err != nil {
		return err
	}
	if err := os.Rename(temp, self); err != nil {
		os.Rename(old, self) // roll back
		return err
	}
	return nil
}
