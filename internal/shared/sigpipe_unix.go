//go:build unix

package shared

import (
	"os/signal"
	"syscall"
)

// IgnoreBrokenPipeSignal makes a write to a closed stdout or stderr return
// EPIPE instead of killing the process. Go's default action for SIGPIPE is to
// terminate, and log.Printf silently discards a write that returns EPIPE - so
// ignoring the signal (rather than catching it on a channel a goroutine could
// leave unread) turns a downstream reader closing early, such as `| head` or a
// restarting log collector, into a dropped log line instead of a dead server.
func IgnoreBrokenPipeSignal() {
	signal.Ignore(syscall.SIGPIPE)
}
