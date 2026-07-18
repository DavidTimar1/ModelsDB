//go:build unix

package selfupdate

import (
	"os"
	"syscall"
)

// Restart re-executes the (now updated) binary in place with execve, replacing
// the current process image. The process keeps its PID, working directory, and
// environment; Go's listening sockets are close-on-exec, so the bound port is
// released for the new image to rebind. On success this call never returns; a
// non-nil error means the exec itself failed and the old image keeps running.
func Restart() error {
	return syscall.Exec(executablePath(), os.Args, os.Environ())
}
