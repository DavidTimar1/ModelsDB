//go:build windows

package shared

// IgnoreBrokenPipeSignal is a no-op on Windows: there is no SIGPIPE there, and
// a write to a closed pipe already surfaces as an error return rather than a
// signal that kills the process.
func IgnoreBrokenPipeSignal() {}
