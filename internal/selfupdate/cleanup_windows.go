//go:build windows

package selfupdate

import (
	"os"
	"time"
)

// CleanupOnStartup removes a leftover "<exe>.old" - the previous binary that a
// Windows in-place update renamed aside so the new one could take its place.
//
// Timing note: right after a self-update restart the OLD process may still be
// exiting and holding "<exe>.old" open, so the first delete can fail with a
// sharing violation. This is best-effort: it deletes immediately if possible,
// otherwise retries briefly in the background. If it still cannot be removed
// (the old process outlives the retries), a subsequent startup - once that
// process has fully exited - removes it. It is a silent no-op when none exists.
func CleanupOnStartup() {
	old := executablePath() + ".old"
	if err := os.Remove(old); err == nil || os.IsNotExist(err) {
		return
	}
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			if err := os.Remove(old); err == nil || os.IsNotExist(err) {
				return
			}
		}
	}()
}
