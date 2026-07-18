// Package shared - single-instance enforcement. Only one ModelsDB server may run
// per data dir at a time: it owns the SQLite DB, which must never be opened or
// migrated by two processes at once. On startup, if the recorded instance still
// holds its port, it is stopped and this one takes over.
//
// Two servers on DIFFERENT data dirs are fine and expected - a sandbox instance
// (see start_dev_agent.sh) runs alongside the normal dev server. Each data dir has
// its own PID file, so they never see each other.
package shared

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/paths"
	"net"
	"os"
	"strings"
	"time"
)

// Instance is what the PID file records about the running server. It carries the
// port and executable, not just a PID, because a bare PID cannot be validated: PIDs
// are recycled, so a stale file can name a live process that is not ours, and
// killing it would hit an unrelated program.
type Instance struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Exe  string `json:"exe"`
}

func PidFilePath() string {
	return paths.PidFile
}

// ReadInstance returns the instance recorded in the PID file. ok is false when the
// file is missing, empty, or unparseable. It does not check whether that process is
// alive - pair it with PortInUse to decide whether a server is actually running.
func ReadInstance() (inst Instance, ok bool) {
	b, err := os.ReadFile(PidFilePath())
	if err != nil {
		return Instance{}, false
	}
	if err := json.Unmarshal(b, &inst); err != nil {
		return Instance{}, false
	}
	if inst.PID <= 0 {
		return Instance{}, false
	}
	return inst, true
}

// PortInUse reports whether the port is currently bound on loopback (i.e. another
// server is likely running).
func PortInUse(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

// KillPreviousInstance stops the recorded instance so this process can take over
// its data dir. Best-effort and cross-platform (os.Process.Kill is SIGKILL on Unix,
// TerminateProcess on Windows).
//
// It kills ONLY when the recorded port is still bound. A PID file whose port is free
// describes an instance that has already exited, and its PID may since have been
// reused by an unrelated program - the port being held is what makes it credible
// that the recorded PID is still ours.
func KillPreviousInstance() {
	inst, ok := ReadInstance()
	if !ok || inst.PID == os.Getpid() {
		return
	}

	// A free port means the recorded instance is gone. Do not signal the PID: it may
	// belong to something else entirely by now.
	if !PortInUse(inst.Port) {
		if err := os.Remove(PidFilePath()); err == nil {
			log.Printf("Cleared stale PID file (recorded pid %d, port %d not bound)", inst.PID, inst.Port)
		}
		return
	}

	proc, err := os.FindProcess(inst.PID)
	if err != nil {
		return // not running (Windows returns an error for a dead PID)
	}
	if err := proc.Kill(); err != nil {
		return // already gone (Unix Kill errors when the process has exited)
	}
	log.Printf("Stopped previous instance (PID %d, port %d)", inst.PID, inst.Port)

	// Wait on the port the OLD instance held, which is not necessarily the port this
	// one wants. The wait exists so the DB is released before we open it; waiting on
	// our own port would return at once whenever the two differ, and we would open a
	// DB the dying process still has.
	for i := 0; i < 50 && PortInUse(inst.Port); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if PortInUse(inst.Port) {
		Fatalf("Stopped pid %d but port %d is still bound after 5s. Another process may hold it; free it and retry.", inst.PID, inst.Port)
	}
}

// WriteInstance records this process so a later start can find and stop it.
func WriteInstance() {
	exe, _ := os.Executable()
	b, err := json.Marshal(Instance{PID: os.Getpid(), Port: paths.Port, Exe: exe})
	if err != nil {
		log.Printf("Warning: failed to encode PID file: %v", err)
		return
	}
	if err := os.WriteFile(PidFilePath(), b, 0644); err != nil {
		log.Printf("Warning: failed to write PID file: %v", err)
	}
}

// Describe renders an instance for an operator message.
func (i Instance) Describe() string {
	exe := i.Exe
	if exe == "" {
		exe = "unknown executable"
	}
	// A `go run` server lives in the build cache; say so, since that path is opaque.
	if strings.Contains(exe, "go-build") {
		exe = exe + " (go run)"
	}
	return fmt.Sprintf("pid %d on port %d (%s)", i.PID, i.Port, exe)
}
