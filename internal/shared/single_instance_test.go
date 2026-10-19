package shared

import (
	"encoding/json"
	"modelsdb/internal/paths"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// isolate points the PID file, the data dir and the port at throwaway values, so
// no test here can read, write, or signal anything belonging to a real install.
// The port matters as much as the paths: the default 8122 is somebody's running
// server, and this package's job is stopping processes.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldPid, oldData, oldPort := paths.PidFile, paths.DataDir, paths.Port
	paths.PidFile = filepath.Join(dir, "modelsdb.pid")
	paths.DataDir = dir
	paths.Port = freePort(t)
	t.Cleanup(func() { paths.PidFile, paths.DataDir, paths.Port = oldPid, oldData, oldPort })
	return dir
}

// freePort returns a loopback port nothing is listening on. It is used only for
// the "nothing is here" cases; a test that needs a real listener takes its port
// from the listener it started.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot reserve a port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// fakeServer starts a loopback HTTP server that answers /api/health with body,
// and returns the port it listens on. It stands in for a running ModelsDB
// instance (or, with a different body, for a foreign process on the port).
func fakeServer(t *testing.T, status int, body map[string]interface{}) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("cannot read the test server's port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	return port
}

func writeRecord(t *testing.T, inst Instance) {
	t.Helper()
	b, err := json.Marshal(inst)
	if err != nil {
		t.Fatalf("cannot encode the record: %v", err)
	}
	if err := os.WriteFile(paths.PidFile, b, 0644); err != nil {
		t.Fatalf("cannot write the record: %v", err)
	}
}

// A free port holds nothing, whatever the PID file claims.
func TestInspectPortReportsNothingWhenThePortIsFree(t *testing.T) {
	isolate(t)
	writeRecord(t, Instance{PID: os.Getpid(), Port: 1, Exe: "x"})

	h := InspectPort(freePort(t))
	if h.Bound || h.Known || h.Ours {
		t.Fatalf("a free port was reported as held: %+v", h)
	}
}

// The server's own answer identifies it, even with no PID file at all - this is
// what lets `stop` end a server whose record was lost.
func TestInspectPortBelievesTheServerOverAMissingRecord(t *testing.T) {
	dir := isolate(t)
	port := fakeServer(t, http.StatusOK, map[string]interface{}{
		"app": HealthAppMarker, "status": "ok", "pid": 4321, "data": dir,
	})

	h := InspectPort(port)
	if !h.Bound || !h.Known || !h.Ours {
		t.Fatalf("the server's own answer was not accepted: %+v", h)
	}
	if h.Source != "server" {
		t.Errorf("source = %q, want \"server\"", h.Source)
	}
	if h.Inst.PID != 4321 {
		t.Errorf("pid = %d, want the one the server reported (4321)", h.Inst.PID)
	}
}

// A stale record naming a dead process must not win over the live server.
func TestInspectPortBelievesTheServerOverAStaleRecord(t *testing.T) {
	dir := isolate(t)
	port := fakeServer(t, http.StatusOK, map[string]interface{}{
		"app": HealthAppMarker, "status": "ok", "pid": 4321, "data": dir,
	})
	writeRecord(t, Instance{PID: 999999, Port: port, Exe: "old"})

	h := InspectPort(port)
	if !h.Ours || h.Inst.PID != 4321 {
		t.Fatalf("the stale record won over the live server: %+v", h)
	}
}

// A server still starting up answers 503 with the same identity block. It must
// still be recognised: it already holds the port and the database.
func TestInspectPortRecognisesAServerThatIsStillStarting(t *testing.T) {
	dir := isolate(t)
	port := fakeServer(t, http.StatusServiceUnavailable, map[string]interface{}{
		"app": HealthAppMarker, "status": "starting", "pid": 4321, "data": dir,
	})

	h := InspectPort(port)
	if !h.Ours || h.Inst.PID != 4321 {
		t.Fatalf("a starting server was not recognised: %+v", h)
	}
}

// Another install on the same port is somebody else's server. Report it, never
// stop it.
func TestInspectPortRefusesAServerForAnotherDataDir(t *testing.T) {
	isolate(t)
	other := t.TempDir()
	port := fakeServer(t, http.StatusOK, map[string]interface{}{
		"app": HealthAppMarker, "status": "ok", "pid": 4321, "data": other,
	})

	h := InspectPort(port)
	if !h.Known {
		t.Fatalf("a ModelsDB server was not identified: %+v", h)
	}
	if h.Ours {
		t.Fatalf("a server for another data dir was treated as ours: %+v", h)
	}
	if h.Why == "" || !strings.Contains(h.Why, other) {
		t.Errorf("the reason does not name the other data dir: %q", h.Why)
	}
}

// A process that is not ModelsDB is never ours, however plausible its reply.
func TestInspectPortRefusesAForeignProcess(t *testing.T) {
	isolate(t)
	port := fakeServer(t, http.StatusOK, map[string]interface{}{
		"status": "ok", "pid": 4321,
	})

	h := InspectPort(port)
	if h.Known || h.Ours {
		t.Fatalf("a foreign process was treated as ModelsDB: %+v", h)
	}
	if h.Why == "" {
		t.Error("no reason was given for refusing a foreign process")
	}
}

// When nothing answers, the record for this very port is still trusted - that is
// how a wedged server that no longer replies is identified.
func TestInspectPortFallsBackToTheRecordWhenNothingAnswers(t *testing.T) {
	isolate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	writeRecord(t, Instance{PID: 4321, Port: port, Exe: "wedged"})

	h := InspectPort(port)
	if !h.Ours || h.Source != "record" || h.Inst.PID != 4321 {
		t.Fatalf("the record was not used as the fallback: %+v", h)
	}
}

// The record is removed only by the process it names.
func TestClearOwnRecordRemovesOnlyThisProcessRecord(t *testing.T) {
	isolate(t)

	WriteInstance()
	if _, ok := ReadInstance(); !ok {
		t.Fatal("WriteInstance wrote no readable record")
	}
	ClearOwnRecord()
	if _, err := os.Stat(paths.PidFile); !os.IsNotExist(err) {
		t.Errorf("this process's own record survived ClearOwnRecord (err = %v)", err)
	}

	// A record naming another process is left alone.
	WriteInstance()
	writeRecord(t, Instance{PID: os.Getpid() + 1, Port: 1, Exe: "someone-else"})
	ClearOwnRecord()
	if _, err := os.Stat(paths.PidFile); err != nil {
		t.Errorf("another process's record was removed: %v", err)
	}
}

// WriteInstance records the data dir, so a probe can tell two installs apart.
func TestWriteInstanceRecordsTheDataDir(t *testing.T) {
	dir := isolate(t)
	WriteInstance()
	t.Cleanup(ClearOwnRecord)

	inst, ok := ReadInstance()
	if !ok {
		t.Fatal("no record was written")
	}
	if inst.Data != dir {
		t.Errorf("recorded data dir = %q, want %q", inst.Data, dir)
	}
	if inst.PID != os.Getpid() {
		t.Errorf("recorded pid = %d, want this process (%d)", inst.PID, os.Getpid())
	}
}

// A relative path and the absolute path of the same folder name one directory.
func TestSameDirResolvesRelativePaths(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !sameDir(".", wd) {
		t.Errorf("%q and the working directory %q were treated as different folders", ".", wd)
	}
	if sameDir("./not-this-one", wd) {
		t.Error("a subfolder was treated as the working directory")
	}
}
