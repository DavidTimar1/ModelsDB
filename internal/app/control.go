// Process control: the interactive menu and the start/stop/status
// subcommands that manage a background server. These let a released user run the
// server in the background without any external scripts. The menu appears when
// the binary is launched in a terminal with no subcommand; the subcommands
// (serve/start/stop/status) are the script-friendly equivalents.

package app

import (
	"bufio"
	"fmt"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
)

// menuIn is the single buffered reader for all interactive menu/prompt input.
// One shared reader is essential: a bufio.Reader over a pipe reads ahead in
// chunks, so a second reader on os.Stdin would miss input the first one already
// buffered. Every menu and sub-prompt reads through this one.
var menuIn = bufio.NewReader(os.Stdin)

// isInteractive reports whether standard input is a terminal, i.e. a human is
// available to answer the menu. A non-terminal stdin (a pipe, a service, or
// nohup with stdin redirected) means no-subcommand launches must start the
// server directly rather than block on a prompt.
func isInteractive() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// serveArgs builds the argument list for a spawned foreground server child:
// the "serve" subcommand plus the same global dir/port/host flags this process
// was launched with, so the child resolves the identical paths and bind address.
func serveArgs() []string {
	return append([]string{"serve"}, paths.GlobalFlagArgs()...)
}

// loopbackURL is the local address a human on this machine uses to reach the UI.
func loopbackURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", paths.Port)
}

// backgroundWatch is how long a background start watches its child before it
// reports success. A start that cannot go on (a database below the schema floor,
// a port taken between the check and the bind, a bad setting) ends the child
// within a moment of launch, long before this passes, so a child still alive
// after it is serving or is migrating a large database. The wait never covers the
// catalog refresh: the child answers 503 "starting" for minutes after the port is
// bound, and that is a healthy start.
const backgroundWatch = 3 * time.Second

// logLineStamp matches the date, time and source position the standard logger
// puts in front of a line, so the text of a failed start reads as the foreground
// start prints it.
var logLineStamp = regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \S+:\d+: `)

// startBackground launches the server as a detached child process and returns to
// the shell. It refuses if a server is already bound to the port. The child runs
// "serve" and writes its own PID file, so stop/status find it. It reports whether
// the start FAILED: a child that ends within backgroundWatch is a failed start,
// and the reason the child logged is printed here, because the child's own output
// goes to the log file and a person told "started" would never open it.
func startBackground() (failed bool) {
	if shared.PortInUse(paths.Port) {
		fmt.Printf("A server is already running on port %d. Use 'stop' first, or 'status' to check.\n", paths.Port)
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Cannot locate this executable to relaunch it: %v\n", err)
		return true
	}

	logPath := filepath.Join(paths.LogsDir, "modelsdb.log")
	logStart := logSize(logPath)

	child := exec.Command(exe, serveArgs()...)
	child.Env = os.Environ()
	child.SysProcAttr = detachSysProcAttr()
	// The child tees its own output to the rotating log file, so its stdio can go
	// to the null device - it must not hold the launching terminal open.
	if devnull, derr := os.OpenFile(os.DevNull, os.O_RDWR, 0); derr == nil {
		child.Stdin, child.Stdout, child.Stderr = devnull, devnull, devnull
		defer devnull.Close()
	}
	if err := child.Start(); err != nil {
		fmt.Printf("Failed to start background server: %v\n", err)
		return true
	}
	pid := child.Process.Pid

	// Wait in the background so the child is reaped whenever it ends and so this
	// process can see an early exit. The wait is never released: Release would
	// hide the exit this function is looking for.
	ended := make(chan error, 1)
	go func() { ended <- child.Wait() }()
	select {
	case waitErr := <-ended:
		fmt.Printf("ModelsDB did not start: the background server (pid %d) ended right after launch.\n", pid)
		if text := logSince(logPath, logStart); text != "" {
			fmt.Println()
			fmt.Println(text)
		} else if waitErr != nil {
			fmt.Printf("  It reported: %v\n", waitErr)
		}
		fmt.Printf("  Full log: %s\n", logPath)
		return true
	case <-time.After(backgroundWatch):
	}

	fmt.Printf("Started ModelsDB in the background (pid %d).\n", pid)
	fmt.Printf("  UI:   %s\n", loopbackURL())
	fmt.Printf("  Logs: %s\n", logPath)
	fmt.Printf("  Stop: %s stop\n", filepath.Base(exe))
	return false
}

// logSize is the current size of the log file, or 0 when there is none yet.
func logSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// logSince returns what the log gained after offset, without the logger's
// per-line stamp, trimmed. A file smaller than offset was rotated, so it is
// read from the start.
func logSince(path string, offset int64) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if offset > int64(len(data)) {
		offset = 0
	}
	text := logLineStamp.ReplaceAllString(string(data[offset:]), "")
	return strings.TrimSpace(text)
}

// stopBackground stops the running server. It asks the PORT who is serving
// there - the server names its own process in its health reply - and falls back
// to the PID file only when nothing answers. That ordering is the point: a
// server must always be stoppable while it is demonstrably serving, whatever the
// recorded file happens to say. When the port is held by something this install
// may not stop, it says exactly what it found and stops nothing.
func stopBackground() {
	h := shared.InspectPort(paths.Port)
	if !h.Bound {
		fmt.Printf("No server is running on port %d.\n", paths.Port)
		// A record left by a start that died names nothing that serves. Clear it,
		// so the next status or start is not misled by it.
		if inst, ok := shared.ReadInstance(); ok && inst.Port == paths.Port {
			if err := os.Remove(shared.PidFilePath()); err == nil {
				fmt.Printf("Cleared a leftover PID file that recorded %s.\n", inst.Describe())
			}
		}
		return
	}
	if !h.Ours {
		fmt.Printf("%s. Not stopping it.\n", h.Why)
		return
	}
	if h.Source == "server" {
		if inst, ok := shared.ReadInstance(); !ok || inst.PID != h.Inst.PID {
			fmt.Printf("The PID file does not name the server holding port %d; using the server's own answer (%s).\n", paths.Port, h.Inst.Describe())
		}
	}
	proc, err := os.FindProcess(h.Inst.PID)
	if err != nil {
		fmt.Printf("Process %d is not available: %v\n", h.Inst.PID, err)
		return
	}
	if err := proc.Kill(); err != nil {
		fmt.Printf("Could not stop process %d: %v\n", h.Inst.PID, err)
		return
	}
	if !shared.WaitForPortFree(paths.Port, 5*time.Second) {
		fmt.Printf("Sent stop to pid %d, but port %d is still busy.\n", h.Inst.PID, paths.Port)
		return
	}
	_ = os.Remove(shared.PidFilePath())
	fmt.Printf("Stopped ModelsDB (pid %d).\n", h.Inst.PID)
}

// printStatus reports whether a server is running and where it is reachable. When
// offerOpen is true (the interactive menu) and a server is running, it offers to
// open the UI in the default browser.
func printStatus(offerOpen bool) {
	h := shared.InspectPort(paths.Port)
	if !h.Bound {
		fmt.Printf("ModelsDB is NOT running (port %d is free).\n", paths.Port)
		return
	}
	if h.Ours {
		fmt.Printf("ModelsDB is RUNNING: %s.\n", h.Inst.Describe())
	} else {
		fmt.Printf("%s.\n", h.Why)
	}
	fmt.Println("  Addresses:")
	for _, addr := range bindAddrs(paths.Host, paths.Port) {
		fmt.Printf("    http://%s\n", addr)
	}
	if offerOpen && promptYesNo("Open the UI in your browser now?") {
		if err := openBrowser(loopbackURL()); err != nil {
			fmt.Printf("  Could not open a browser: %v\n", err)
		}
	}
}

// runUpdateChild refreshes the model catalog by running the existing "update"
// subcommand as a child process, so the menu process never opens the database
// itself. It refuses while a server is running, because that server owns the DB
// and exposes the in-app Update button for the same job.
func runUpdateChild() {
	if shared.PortInUse(paths.Port) {
		fmt.Printf("A server is running on port %d. Use the Update button in the web UI (%s) instead.\n", paths.Port, loopbackURL())
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Cannot locate this executable: %v\n", err)
		return
	}
	child := exec.Command(exe, append([]string{"update"}, paths.GlobalFlagArgs()...)...)
	child.Env = os.Environ()
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	fmt.Println("Updating models...")
	if err := child.Run(); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}
	fmt.Println("Update complete.")
}

// openBrowser opens url in the platform default browser without blocking.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// promptYesNo asks a yes/no question on the terminal, defaulting to no.
func promptYesNo(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	line, _ := menuIn.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// menuRule prints a horizontal divider with surrounding blank lines. It runs
// between a finished action's output and the next menu redraw, so the fresh
// output stands apart from what scrolled past before it instead of merging into
// one wall of text.
func menuRule() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
}

// runMenu drives the interactive process-control menu. It loops until the user
// starts the server in the foreground (which takes over this process) or quits.
func runMenu() {
	first := true
	for {
		state := "stopped"
		if h := shared.InspectPort(paths.Port); h.Bound {
			if h.Known {
				state = fmt.Sprintf("running, pid %d", h.Inst.PID)
			} else {
				state = "port taken"
			}
		}
		if first {
			fmt.Println()
			first = false
		} else {
			menuRule()
		}
		fmt.Printf("ModelsDB %s  [%s]  port %d\n", shared.Version, state, paths.Port)
		fmt.Println("  1) Start in background")
		fmt.Println("  2) Stop background instance")
		fmt.Println("  3) Start normally (foreground)")
		fmt.Println("  4) Status / open in browser")
		fmt.Println("  5) Update models now")
		fmt.Println("  6) Add to PATH (run 'modelsdb' from anywhere)")
		fmt.Println("  7) Quit")
		fmt.Print("Choose [1-7]: ")

		line, err := menuIn.ReadString('\n')
		if err != nil { // EOF (e.g. stdin closed) - nothing more to read
			fmt.Println()
			return
		}
		switch strings.TrimSpace(line) {
		case "1":
			startBackground() // prints its own failure; the menu carries on
		case "2":
			stopBackground()
		case "3":
			fmt.Println("Starting in the foreground - press Ctrl-C to stop.")
			StartServer() // blocks; on Ctrl-C the process exits (does not return here)
			return
		case "4":
			printStatus(true)
		case "5":
			runUpdateChild()
		case "6":
			runPathInstall()
		case "7", "q", "quit", "exit":
			return
		default:
			fmt.Println("Enter a number from 1 to 7.")
		}
	}
}
