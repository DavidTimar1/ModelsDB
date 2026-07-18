//go:build !windows

package selfupdate

// CleanupOnStartup is a no-op on unix. The in-place update renames the new
// binary directly over the executable, so nothing is left behind to clean up.
func CleanupOnStartup() {}
