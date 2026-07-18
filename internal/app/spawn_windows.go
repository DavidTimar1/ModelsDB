//go:build windows

package app

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// detachSysProcAttr returns the creation flags that fully detach a spawned child
// from this console, so a background server keeps running after the launching
// window closes. DETACHED_PROCESS gives the child no console, CREATE_NO_WINDOW
// suppresses a new window, and CREATE_NEW_PROCESS_GROUP isolates it from
// Ctrl-C/Ctrl-Break sent to the launcher.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
