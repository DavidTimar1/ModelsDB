// Fatal-error handling that stays visible.
//
// A double-clicked console app whose process exits closes its window
// immediately, hiding any error. These helpers log the error and, when running
// interactively (a real console / TTY on stdin and stderr), wait for Enter before exiting
// so the message is readable. When stdin is not a TTY (scripts, pipes, CI,
// background runs) they exit immediately - so automation never hangs.

package shared

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/mattn/go-isatty"
)

// isInteractive reports whether a person can read the prompt and answer it: both
// stdin and stderr (where the prompt is written) are terminals. A character
// device is not enough, because /dev/null is one and a script that redirects
// stdin from it must never be made to wait.
func isInteractive() bool {
	return isTerminal(os.Stdin.Fd()) && isTerminal(os.Stderr.Fd())
}

func isTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func pauseIfInteractive() {
	if isInteractive() {
		fmt.Fprint(os.Stderr, "\nPress Enter to close this window...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
}

// Fatal logs a fatal error, pauses if interactive, then exits non-zero. It takes
// away this process's PID-file record first (see ClearOwnRecord), because a
// process on its way out is no longer serving and must stop being recorded as
// the server - and it does so before the pause, so the record goes even if
// nobody ever presses Enter.
func Fatal(v ...interface{}) {
	log.Print(v...)
	ClearOwnRecord()
	pauseIfInteractive()
	os.Exit(1)
}

// Fatalf is Fatal with a format string.
func Fatalf(format string, v ...interface{}) {
	log.Printf(format, v...)
	ClearOwnRecord()
	pauseIfInteractive()
	os.Exit(1)
}

// RecoverPanic turns an unhandled panic into a visible message + pause. Call it
// deferred at the top of the entry function.
func RecoverPanic() {
	if r := recover(); r != nil {
		log.Printf("FATAL: unexpected error (panic): %v", r)
		ClearOwnRecord()
		pauseIfInteractive()
		os.Exit(1)
	}
}
