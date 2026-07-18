// Package shared - fatal-error handling that stays visible.
//
// A double-clicked console app whose process exits closes its window
// immediately, hiding any error. These helpers log the error and, when running
// interactively (a real console / TTY on stdin), wait for Enter before exiting
// so the message is readable. When stdin is not a TTY (scripts, pipes, CI,
// background runs) they exit immediately - so automation never hangs.
package shared

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func pauseIfInteractive() {
	if isInteractive() {
		fmt.Fprint(os.Stderr, "\nPress Enter to close this window...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
}

// Fatal logs a fatal error, pauses if interactive, then exits non-zero.
func Fatal(v ...interface{}) {
	log.Print(v...)
	pauseIfInteractive()
	os.Exit(1)
}

// Fatalf is Fatal with a format string.
func Fatalf(format string, v ...interface{}) {
	log.Printf(format, v...)
	pauseIfInteractive()
	os.Exit(1)
}

// RecoverPanic turns an unhandled panic into a visible message + pause. Call it
// deferred at the top of the entry function.
func RecoverPanic() {
	if r := recover(); r != nil {
		log.Printf("FATAL: unexpected error (panic): %v", r)
		pauseIfInteractive()
		os.Exit(1)
	}
}
