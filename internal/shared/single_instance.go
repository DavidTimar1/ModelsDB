// Single-instance enforcement. Only one ModelsDB server may run
// per data dir at a time: it owns the SQLite DB, which must never be opened or
// migrated by two processes at once. On startup, if the recorded instance still
// holds its port, it is stopped and this one takes over.
//
// Two servers on DIFFERENT data dirs are fine and expected - a sandbox instance
// (see start_dev_agent.sh) runs alongside the normal dev server. Each data dir has
// its own PID file, so they never see each other.
//
// The PID file is a RECORD OF A LISTENER, never of a hopeful start. The server
// takes its port first and writes the record only once the port is its own, so a
// start that cannot bind leaves the record of the healthy server that holds the
// port exactly as it found it. A controlled exit after that (a failed database
// open, a panic) removes the record again through ClearOwnRecord, so the file
// names a process that is listening or it names nothing.
//
// Who holds the port is answered by InspectPort, which asks TWO sources in order:
// the server itself, over GET /api/health on loopback (it reports its own process
// id and data dir, so it is right even when the file is wrong), and then the PID
// file. That is what lets `stop` end a server whose record was lost.

package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/paths"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// healthProbeTimeout bounds the GET /api/health question put to whatever holds
// the port. A ModelsDB server on loopback answers in milliseconds; anything else
// must not stall a stop, a status, or a startup takeover.
const healthProbeTimeout = 2 * time.Second

// portReleaseWait is how long a takeover waits for a stopped instance to let go
// of its port (and with it the database file) before giving up.
const portReleaseWait = 5 * time.Second

// Instance is what identifies one running server. It carries the port, data dir
// and executable, not just a PID, because a bare PID cannot be validated: PIDs are
// recycled, so a stale file can name a live process that is not ours, and killing
// it would hit an unrelated program.
type Instance struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Exe  string `json:"exe"`
	Data string `json:"data,omitempty"` // the data dir this server owns
}

// PidFilePath is where this install's single-instance record lives.
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

// PortHolder is what was learned about whatever holds a port: whether anything
// holds it, whether that thing is a ModelsDB server, and whether it is THIS
// install's server - the only case in which it may be stopped.
type PortHolder struct {
	Bound  bool     // something is listening on the port
	Inst   Instance // what was learned about it (zero when nothing was)
	Known  bool     // it is a ModelsDB server, identified well enough to stop
	Ours   bool     // ...and it serves this install's data dir
	Source string   // where Inst came from: "server" (it answered) or "record"
	Why    string   // when Ours is false: the reason, phrased for an operator
}

// InspectPort identifies whatever is listening on port. It asks the server first
// and the PID file second, because a server that answers /api/health reports its
// OWN process id and data dir, which is true even when the recorded file is stale,
// missing, or was overwritten by a start that never served.
//
// A holder that answers as ModelsDB for a DIFFERENT data dir is deliberately not
// ours: two installs on one port is a misconfiguration to report, never a process
// to kill. A holder that does not answer at all falls back to the record, which is
// trusted only when it names this very port - the same evidence the app has always
// acted on, and what still identifies a server too wedged to reply.
func InspectPort(port int) PortHolder {
	h := PortHolder{Bound: PortInUse(port)}
	if !h.Bound {
		return h
	}

	if inst, ok := probeHealth(port); ok {
		h.Inst, h.Known, h.Source = inst, true, "server"
		if inst.Data != "" && paths.DataDir != "" && !sameDir(inst.Data, paths.DataDir) {
			h.Why = fmt.Sprintf("Port %d is held by a ModelsDB server for a different data dir (%s), pid %d", port, inst.Data, inst.PID)
			return h
		}
		h.Ours = true
		return h
	}

	inst, ok := ReadInstance()
	switch {
	case !ok:
		h.Why = fmt.Sprintf("Port %d is held by a process that does not answer as ModelsDB, and no PID file records one", port)
	case inst.Port != port:
		h.Inst = inst
		h.Why = fmt.Sprintf("Port %d is held by a process that does not answer as ModelsDB; the PID file records %s", port, inst.Describe())
	default:
		h.Inst, h.Known, h.Ours, h.Source = inst, true, true, "record"
	}
	return h
}

// probeHealth asks whatever holds the port to identify itself over
// GET /api/health on loopback. A server answers with its own process id even
// while it is still starting up (that reply carries a 503 status, so the body is
// read whatever the status is). Anything that is not a ModelsDB server fails the
// "app" marker and is reported as unidentified.
func probeHealth(port int) (Instance, bool) {
	client := &http.Client{Timeout: healthProbeTimeout}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return Instance{}, false
	}
	defer resp.Body.Close()

	// Read a bounded amount: a foreign process on this port could stream forever.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Instance{}, false
	}
	var reply struct {
		App  string `json:"app"`
		PID  int    `json:"pid"`
		Port int    `json:"port"`
		Exe  string `json:"exe"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return Instance{}, false
	}
	if reply.App != HealthAppMarker || reply.PID <= 0 {
		return Instance{}, false
	}
	return Instance{PID: reply.PID, Port: reply.Port, Exe: reply.Exe, Data: reply.Data}, true
}

// HealthAppMarker is the "app" value a ModelsDB server puts in its health reply.
// The probe requires it before it will treat the process holding a port as ours,
// because the next thing done with that process id is to end the process.
const HealthAppMarker = "modelsdb"

// sameDir reports whether two directory paths name the same directory, comparing
// them the way the filesystem does: exactly on Linux, ignoring case on Windows
// and macOS. Both are made absolute first, because a server reports its data dir
// absolutely while a command given a relative --data-dir holds it as typed.
func sameDir(a, b string) bool {
	a, b = absClean(a), absClean(b)
	if runtime.GOOS == "linux" {
		return a == b
	}
	return strings.EqualFold(a, b)
}

// absClean returns the cleaned absolute form of a path, or just the cleaned path
// when the working directory cannot be read.
func absClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// KillPreviousInstance stops any earlier server of this install so this process
// can take over its data dir, and returns once nothing else holds the database.
// Best-effort and cross-platform (os.Process.Kill is SIGKILL on Unix,
// TerminateProcess on Windows).
//
// It looks in two places, because either can be the only one that knows:
//
//  1. The PID file, on WHATEVER port it recorded. That instance owns this data
//     dir even when this run was given a different port, so it has to go. The
//     process stopped is the one that port's health answer names, never the
//     recorded PID by itself: PIDs are recycled, so a record can name a live
//     process that is not ours, and a free port means the instance has already
//     exited, so the record is cleared and nothing is signalled.
//  2. The port this run wants. A server can be serving here with no record naming
//     it (a record lost, or overwritten by an older build's start that died), and
//     it is stopped only when it identifies itself as ModelsDB on this data dir.
//
// A port held by something this process may not stop is left alone: the bind
// that follows fails with a precise error, which is the honest outcome.
func KillPreviousInstance() {
	if inst, ok := ReadInstance(); ok && inst.PID != os.Getpid() {
		if PortInUse(inst.Port) {
			if live, stop := recordedServerToStop(inst); stop {
				stopInstance(live)
			}
		} else if err := os.Remove(PidFilePath()); err == nil {
			log.Printf("Cleared stale PID file (recorded pid %d, port %d not bound)", inst.PID, inst.Port)
		}
	}

	if h := InspectPort(paths.Port); h.Bound && h.Ours && h.Inst.PID != os.Getpid() {
		log.Printf("Port %d is held by a ModelsDB server the PID file does not name (%s)", paths.Port, h.Inst.Describe())
		stopInstance(h.Inst)
	}
}

// recordedServerToStop decides whether the process holding the port a PID file
// recorded may be stopped, and which process that is. Only a server that answers
// GET /api/health as ModelsDB for this data dir qualifies, and the process
// returned is the one that answered: the recorded PID is not trusted on its own,
// because it may have been reused by an unrelated program that now holds the port.
func recordedServerToStop(recorded Instance) (Instance, bool) {
	live, answered := probeHealth(recorded.Port)
	if !answered {
		log.Printf("Port %d is held by a process that does not answer as ModelsDB; the PID file records pid %d, which is left alone", recorded.Port, recorded.PID)
		return Instance{}, false
	}
	if live.Data != "" && paths.DataDir != "" && !sameDir(live.Data, paths.DataDir) {
		log.Printf("Port %d is held by a ModelsDB server for another data dir (%s); the PID file's pid %d is left alone", recorded.Port, live.Data, recorded.PID)
		return Instance{}, false
	}
	if live.PID <= 0 || live.PID == os.Getpid() {
		return Instance{}, false
	}
	if live.Port == 0 {
		live.Port = recorded.Port
	}
	return live, true
}

// stopInstance ends one instance and waits for it to release the port it held -
// which is what proves it has let go of the database file. The wait is on THAT
// instance's port, not this run's: waiting on a port the dying process never held
// would return at once and prove nothing.
func stopInstance(inst Instance) {
	proc, err := os.FindProcess(inst.PID)
	if err != nil {
		return // not running (Windows returns an error for a dead PID)
	}
	if err := proc.Kill(); err != nil {
		return // already gone (Unix Kill errors when the process has exited)
	}
	log.Printf("Stopped previous instance (PID %d, port %d)", inst.PID, inst.Port)

	if !WaitForPortFree(inst.Port, portReleaseWait) {
		Fatalf("Stopped pid %d but port %d is still bound after %s. Another process may hold it; free it and retry.", inst.PID, inst.Port, portReleaseWait)
	}
}

// WaitForPortFree waits up to limit for the port to become free, reporting
// whether it did.
func WaitForPortFree(port int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for {
		if !PortInUse(port) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// wroteOwnRecord records that this process put its own PID in the PID file, so a
// controlled exit knows there is a record of its own to take away again. It is
// atomic because a fatal error can be raised from a serving goroutine.
var wroteOwnRecord atomic.Bool

// WriteInstance records this process as the server holding the port, so a later
// start, a stop, or a status can find it. Call it only once the port is actually
// bound: the file must never describe a start that has not served, because
// writing it earlier overwrites the record of a healthy server that is still
// running - which is how an operator ends up unable to stop their own server.
func WriteInstance() {
	exe, _ := os.Executable()
	b, err := json.Marshal(Instance{PID: os.Getpid(), Port: paths.Port, Exe: exe, Data: paths.DataDir})
	if err != nil {
		log.Printf("Warning: failed to encode PID file: %v", err)
		return
	}
	if err := paths.WriteFileAtomic(PidFilePath(), b, 0644); err != nil {
		log.Printf("Warning: failed to write PID file: %v", err)
		return
	}
	wroteOwnRecord.Store(true)
}

// ClearOwnRecord removes the PID file when it names THIS process, so a server
// that stops serving stops being recorded. It is called on every exit this
// program controls (a fatal error, a panic, a stop). A record naming another
// process is left alone: it is not this process's to remove.
func ClearOwnRecord() {
	if !wroteOwnRecord.Load() {
		return
	}
	if inst, ok := ReadInstance(); !ok || inst.PID != os.Getpid() {
		return
	}
	if err := os.Remove(PidFilePath()); err == nil {
		wroteOwnRecord.Store(false)
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
