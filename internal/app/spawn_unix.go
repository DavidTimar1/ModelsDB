//go:build unix

package app

import "syscall"

// detachSysProcAttr returns the attributes that fully detach a spawned child
// from this process's controlling terminal and session, so a background server
// keeps running after the launching shell exits. Setsid starts the child in a
// new session with no controlling TTY.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
