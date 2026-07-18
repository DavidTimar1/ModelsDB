// Package app - process control: the interactive menu and the start/stop/status
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

// startBackground launches the server as a detached child process and returns to
// the shell. It refuses if a server is already bound to the port. The child runs
// "serve" and writes its own PID file, so stop/status find it.
func startBackground() {
	if shared.PortInUse(paths.Port) {
		fmt.Printf("A server is already running on port %d. Use 'stop' first, or 'status' to check.\n", paths.Port)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Cannot locate this executable to relaunch it: %v\n", err)
		return
	}

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
		return
	}
	// Capture the PID before releasing: Release invalidates the Process handle
	// and resets its Pid to -1.
	pid := child.Process.Pid
	// Release so this process does not remain the child's parent/waiter.
	_ = child.Process.Release()

	fmt.Printf("Started ModelsDB in the background (pid %d).\n", pid)
	fmt.Printf("  UI:   %s\n", loopbackURL())
	fmt.Printf("  Logs: %s\n", filepath.Join(paths.LogsDir, "modelsdb.log"))
	fmt.Printf("  Stop: %s stop\n", filepath.Base(exe))
}

// stopBackground stops a running server. It uses the bound port as the "is a
// server running" signal and the PID file to know which process to stop.
func stopBackground() {
	if !shared.PortInUse(paths.Port) {
		fmt.Printf("No server is running on port %d.\n", paths.Port)
		return
	}
	inst, ok := shared.ReadInstance()
	if !ok {
		fmt.Printf("Port %d is in use but no PID file was found - another (non-ModelsDB) process may hold it. Not stopping it.\n", paths.Port)
		return
	}
	// Refuse when the record describes a server on a different port: that instance
	// is not the one holding this port, so its PID is not ours to kill.
	if inst.Port != paths.Port {
		fmt.Printf("Port %d is in use, but the PID file records %s. Not stopping it.\n", paths.Port, inst.Describe())
		return
	}
	proc, err := os.FindProcess(inst.PID)
	if err != nil {
		fmt.Printf("Recorded process %d is not available: %v\n", inst.PID, err)
		return
	}
	if err := proc.Kill(); err != nil {
		fmt.Printf("Could not stop process %d: %v\n", inst.PID, err)
		return
	}
	for i := 0; i < 50 && shared.PortInUse(paths.Port); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if shared.PortInUse(paths.Port) {
		fmt.Printf("Sent stop to pid %d, but port %d is still busy.\n", inst.PID, paths.Port)
		return
	}
	_ = os.Remove(shared.PidFilePath())
	fmt.Printf("Stopped ModelsDB (pid %d).\n", inst.PID)
}

// printStatus reports whether a server is running and where it is reachable. When
// offerOpen is true (the interactive menu) and a server is running, it offers to
// open the UI in the default browser.
func printStatus(offerOpen bool) {
	if !shared.PortInUse(paths.Port) {
		fmt.Printf("ModelsDB is NOT running (port %d is free).\n", paths.Port)
		return
	}
	if inst, ok := shared.ReadInstance(); ok && inst.Port == paths.Port {
		fmt.Printf("ModelsDB is RUNNING: %s.\n", inst.Describe())
	} else {
		fmt.Printf("Port %d is in use (no matching PID file - it may be a non-ModelsDB process).\n", paths.Port)
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

// runMenu drives the interactive process-control menu. It loops until the user
// starts the server in the foreground (which takes over this process) or quits.
// menuRule prints a horizontal divider with surrounding blank lines. It runs
// between a finished action's output and the next menu redraw, so the fresh
// output stands apart from what scrolled past before it instead of merging into
// one wall of text.
func menuRule() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
}

func runMenu() {
	first := true
	for {
		state := "stopped"
		if shared.PortInUse(paths.Port) {
			if inst, ok := shared.ReadInstance(); ok && inst.Port == paths.Port {
				state = fmt.Sprintf("running, pid %d", inst.PID)
			} else {
				state = "running"
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
			startBackground()
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
