//go:build windows

package selfupdate

import (
	"os"
	"os/exec"
)

// Restart launches the updated binary as a fresh process and exits the current
// one. Windows has no execve, so a new process is spawned with the same
// arguments and this one terminates. The new process's single-instance startup
// stops any lingering old instance and rebinds the port. On success this call
// does not return (it exits the process); a non-nil error means the new process
// could not be started and the old one keeps running.
func Restart() error {
	cmd := exec.Command(executablePath(), os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
