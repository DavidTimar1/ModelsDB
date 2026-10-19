package app

import (
	"encoding/json"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guardSandbox points the path variables at a scratch data folder and returns
// a port nothing listens on, restoring everything afterwards.
func guardSandbox(t *testing.T) (dir string, freePort int) {
	t.Helper()
	dir = t.TempDir()
	origPort, origData, origPid := paths.Port, paths.DataDir, paths.PidFile
	t.Cleanup(func() { paths.Port, paths.DataDir, paths.PidFile = origPort, origData, origPid })
	paths.DataDir = dir
	paths.PidFile = filepath.Join(dir, "modelsdb.pid")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	freePort = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	paths.Port = freePort
	return dir, freePort
}

// fakeModelsDB serves a ModelsDB-shaped health reply for the given data dir and
// returns the port it listens on.
func fakeModelsDB(t *testing.T, data string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"app": shared.HealthAppMarker, "pid": 4321, "data": data})
	}))
	t.Cleanup(srv.Close)
	port, err := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func writeRecord(t *testing.T, port int) {
	t.Helper()
	b, _ := json.Marshal(shared.Instance{PID: 4321, Port: port, Data: paths.DataDir})
	if err := os.WriteFile(paths.PidFile, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A listener on the configured port is a server (or something that may be one)
// holding the data folder, so restore and update must refuse; a free port is not.
func TestServerHoldingDataOnResolvedPort(t *testing.T) {
	_, _ = guardSandbox(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	paths.Port = ln.Addr().(*net.TCPAddr).Port
	if port, held := serverHoldingData(); !held || port != paths.Port {
		t.Errorf("a process listening on the resolved port was not treated as holding the data folder (port %d, held %v)", port, held)
	}
	ln.Close()
	if _, held := serverHoldingData(); held {
		t.Error("a free port was treated as a running server")
	}
}

// A server recorded on another port than the one this command resolved still
// holds the data folder, and the answer names the port it really holds.
func TestServerHoldingDataOnRecordedPort(t *testing.T) {
	dir, _ := guardSandbox(t)
	other := fakeModelsDB(t, dir)
	writeRecord(t, other)
	port, held := serverHoldingData()
	if !held || port != other {
		t.Errorf("server recorded on port %d: got port %d, held %v", other, port, held)
	}
}

// A record whose port is free, or held by a ModelsDB for another data folder,
// holds nothing; with no record and a free port nothing runs either.
func TestServerHoldingDataIgnoresStaleRecords(t *testing.T) {
	dir, free := guardSandbox(t)
	if _, held := serverHoldingData(); held {
		t.Error("nothing running was treated as a held data folder")
	}
	writeRecord(t, free+1)
	if _, held := serverHoldingData(); held {
		t.Error("a record whose port is free was treated as a running server")
	}
	foreign := fakeModelsDB(t, filepath.Join(dir, "elsewhere"))
	writeRecord(t, foreign)
	if _, held := serverHoldingData(); held {
		t.Error("a record naming a server for another data folder was treated as holding this one")
	}
}
