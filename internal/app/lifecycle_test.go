package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Server lifecycle, exercised with REAL processes.
//
// The rules these tests hold the app to:
//
//   - The PID file names a process that is listening, or it names nothing. A
//     start that cannot take the port leaves the file it found untouched, and a
//     start that dies after taking it takes its own record away again.
//   - One data dir, one server. A second start takes over and the first exits, so
//     two processes never hold the same database.
//   - Different data dirs are independent: a sandbox instance runs alongside a
//     normal one, and neither notices the other.
//   - `stop` ends a server that is demonstrably serving, even when the recorded
//     PID is stale or the file is gone entirely.
//
// Every server here gets its own throwaway directory and a port the test itself
// reserved, and is ended by its exact process id. Nothing looks at, or listens
// on, a real install's port or directories.

// buildOnce builds the binary these tests drive, once for the whole package.
var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// modelsdbBinary returns the path to a freshly built modelsdb executable. The
// tests drive the real binary because what they check is the ORDER of steps
// inside a live process - bind, then record, then open the database - which no
// in-process call can demonstrate.
func modelsdbBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "modelsdb-lifecycle")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "modelsdb-test-binary")
		cmd := exec.Command("go", "build", "-o", builtBin, "modelsdb/cmd/modelsdb")
		out, cerr := cmd.CombinedOutput()
		if cerr != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", cerr, out)
		}
	})
	if buildErr != nil {
		t.Fatalf("cannot build the binary under test: %v", buildErr)
	}
	return builtBin
}

// reservePort returns a loopback port that was free a moment ago. It is never a
// port a person's install uses: the operating system hands out an ephemeral one.
func reservePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot reserve a port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// sandbox is one throwaway install: its own config, data and cache directory
// (all three the same folder, as a portable install has them) and its own port.
type sandbox struct {
	dir  string
	port int
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	return sandbox{dir: t.TempDir(), port: reservePort(t)}
}

func (s sandbox) pidFile() string { return filepath.Join(s.dir, "modelsdb.pid") }

// args are the global flags that pin a child process to this sandbox. Passing
// all three directories explicitly is what guarantees the child cannot resolve
// onto a real install's folders whatever the environment says.
func (s sandbox) args(cmd string) []string {
	return []string{cmd,
		"--config-dir", s.dir,
		"--data-dir", s.dir,
		"--cache-dir", s.dir,
		"--port", strconv.Itoa(s.port),
	}
}

// childEnv is the environment every child process runs with. It points every
// "where does this user keep things" variable at the sandbox, and sends outbound
// HTTP at a dead proxy so the startup catalog fetch and the update check fail at
// once instead of reaching the internet.
func childEnv(dir string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + dir,
		"USERPROFILE=" + dir,
		"XDG_CONFIG_HOME=" + dir,
		"XDG_DATA_HOME=" + dir,
		"XDG_CACHE_HOME=" + dir,
		"HTTP_PROXY=http://127.0.0.1:1",
		"HTTPS_PROXY=http://127.0.0.1:1",
		"http_proxy=http://127.0.0.1:1",
		"https_proxy=http://127.0.0.1:1",
		"NO_PROXY=127.0.0.1,localhost",
		"no_proxy=127.0.0.1,localhost",
	}
}

// startServer launches a server for the sandbox and waits until it answers as
// healthy. It is ended by its exact process id when the test finishes, whatever
// else the test does.
func startServer(t *testing.T, s sandbox) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(modelsdbBinary(t), s.args("serve")...)
	cmd.Env = childEnv(s.dir)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("cannot start a server: %v", err)
	}
	t.Cleanup(func() { endProcess(cmd) })

	if !waitHealthy(s.port, cmd.Process.Pid, 30*time.Second) {
		t.Fatalf("the server on port %d never became healthy", s.port)
	}
	return cmd
}

// endProcess ends one process by its exact process id and reaps it, so a test
// leaves nothing running. A process that has already exited is left alone.
func endProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

// health asks a port to identify itself and returns the decoded reply. ok is
// false when nothing answered or the answer was not a ModelsDB server.
func health(port int) (reply map[string]interface{}, ok bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, false
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, false
	}
	if reply["app"] != "modelsdb" {
		return nil, false
	}
	return reply, true
}

// waitHealthy waits for the given process to be the one serving on the port,
// fully started.
func waitHealthy(port, pid int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if reply, ok := health(port); ok && reply["status"] == "ok" && int(reply["pid"].(float64)) == pid {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// waitExit waits for a process to exit and returns whether it did.
func waitExit(cmd *exec.Cmd, limit time.Duration) bool {
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

// runControl runs a control subcommand (stop, status) against a sandbox and
// returns its combined output.
func runControl(t *testing.T, s sandbox, cmd string) string {
	t.Helper()
	c := exec.Command(modelsdbBinary(t), s.args(cmd)...)
	c.Env = childEnv(s.dir)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", cmd, err, out)
	}
	return string(out)
}

// deadPID returns the process id of a child that has already exited: the binary
// run with --version, which prints and stops. It stands in for the stale PID a
// crashed instance leaves behind.
func deadPID(t *testing.T) int {
	t.Helper()
	c := exec.Command(modelsdbBinary(t), "--version")
	c.Env = childEnv(t.TempDir())
	if err := c.Run(); err != nil {
		t.Fatalf("cannot produce an exited process: %v", err)
	}
	return c.ProcessState.Pid()
}

// A start that cannot take the port must not touch the PID file. This is the
// defect that cost a maintainer the ability to stop their own server: the old
// order wrote the record first, so a start that then failed to bind left the
// file naming a process that never served, on top of the record of the healthy
// server still holding the port.
func TestAStartThatCannotBindLeavesTheRecordUntouched(t *testing.T) {
	s := newSandbox(t)

	// Something else is on the port, and will not let go.
	holder, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		t.Fatalf("cannot occupy the port: %v", err)
	}
	defer holder.Close()

	// A record is already there, describing an instance on this port.
	before := []byte(fmt.Sprintf(`{"pid":%d,"port":%d,"exe":"previous"}`, deadPID(t), s.port))
	if err := os.WriteFile(s.pidFile(), before, 0644); err != nil {
		t.Fatalf("cannot place the record: %v", err)
	}

	cmd := exec.Command(modelsdbBinary(t), s.args("serve")...)
	cmd.Env = childEnv(s.dir)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("the server started even though the port was taken:\n%s", out)
	}

	after, err := os.ReadFile(s.pidFile())
	if err != nil {
		t.Fatalf("the existing record was removed by a failed start: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("a failed start rewrote the record:\n  before: %s\n  after:  %s", before, after)
	}
}

// A start that takes the port and then dies must take its record away with it,
// so the file never names a process that is not serving.
func TestAStartThatDiesAfterTakingThePortLeavesNoRecord(t *testing.T) {
	s := newSandbox(t)

	// A directory where the database file belongs: the open fails, which happens
	// after the port has been taken and the record written.
	if err := os.MkdirAll(filepath.Join(s.dir, "modelsdb.db"), 0755); err != nil {
		t.Fatalf("cannot block the database file: %v", err)
	}

	cmd := exec.Command(modelsdbBinary(t), s.args("serve")...)
	cmd.Env = childEnv(s.dir)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("the server started with an unopenable database:\n%s", out)
	}

	if _, err := os.Stat(s.pidFile()); !os.IsNotExist(err) {
		rec, _ := os.ReadFile(s.pidFile())
		t.Errorf("a start that died left a record behind: %s (stat err %v)", rec, err)
	}
}

// One data dir, one server: the second start takes over and the first process
// exits, so two processes never hold the same database.
func TestASecondStartOnOneDataDirEndsTheFirst(t *testing.T) {
	s := newSandbox(t)
	first := startServer(t, s)

	second := startServer(t, s)

	if !waitExit(first, 10*time.Second) {
		t.Fatalf("the first server (pid %d) is still running after a second one took over", first.Process.Pid)
	}
	reply, ok := health(s.port)
	if !ok {
		t.Fatal("nothing is serving after the takeover")
	}
	if int(reply["pid"].(float64)) != second.Process.Pid {
		t.Errorf("port %d is served by pid %v, want the second server (pid %d)", s.port, reply["pid"], second.Process.Pid)
	}
	inst := readRecord(t, s)
	if inst.PID != second.Process.Pid {
		t.Errorf("the record names pid %d, want the serving process (pid %d)", inst.PID, second.Process.Pid)
	}
}

// Two data dirs are two independent installs - the everyday arrangement of a
// sandbox instance running alongside a normal one. Neither may disturb the other.
func TestTwoDataDirsRunSideBySide(t *testing.T) {
	one, two := newSandbox(t), newSandbox(t)
	first := startServer(t, one)
	second := startServer(t, two)

	for _, c := range []struct {
		name string
		s    sandbox
		cmd  *exec.Cmd
	}{{"first", one, first}, {"second", two, second}} {
		reply, ok := health(c.s.port)
		if !ok {
			t.Fatalf("the %s server stopped answering on port %d", c.name, c.s.port)
		}
		if int(reply["pid"].(float64)) != c.cmd.Process.Pid {
			t.Errorf("the %s port is served by pid %v, want %d", c.name, reply["pid"], c.cmd.Process.Pid)
		}
		if got := readRecord(t, c.s).PID; got != c.cmd.Process.Pid {
			t.Errorf("the %s record names pid %d, want %d", c.name, got, c.cmd.Process.Pid)
		}
	}
}

// `stop` must end a server that is demonstrably serving, whatever the recorded
// file says - the maintainer's server was left unstoppable precisely because the
// record disagreed with reality.
func TestStopEndsAServerWhoseRecordIsWrong(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, s sandbox)
	}{
		{"record deleted", func(t *testing.T, s sandbox) {
			if err := os.Remove(s.pidFile()); err != nil {
				t.Fatalf("cannot remove the record: %v", err)
			}
		}},
		{"record names a dead process", func(t *testing.T, s sandbox) {
			rec := fmt.Sprintf(`{"pid":%d,"port":%d,"exe":"stale"}`, deadPID(t), s.port)
			if err := os.WriteFile(s.pidFile(), []byte(rec), 0644); err != nil {
				t.Fatalf("cannot damage the record: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSandbox(t)
			server := startServer(t, s)
			tc.damage(t, s)

			out := runControl(t, s, "stop")
			if !strings.Contains(out, "Stopped ModelsDB") {
				t.Fatalf("stop did not stop the server:\n%s", out)
			}
			if !waitExit(server, 10*time.Second) {
				t.Fatalf("stop reported success but pid %d is still running", server.Process.Pid)
			}
			if _, ok := health(s.port); ok {
				t.Error("something is still serving on the port after stop")
			}
		})
	}
}

// readRecord reads the sandbox's PID file, failing the test when it is absent or
// unreadable.
func readRecord(t *testing.T, s sandbox) struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Data string `json:"data"`
} {
	t.Helper()
	var rec struct {
		PID  int    `json:"pid"`
		Port int    `json:"port"`
		Data string `json:"data"`
	}
	b, err := os.ReadFile(s.pidFile())
	if err != nil {
		t.Fatalf("no PID file: %v", err)
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("the PID file is not readable JSON (%s): %v", b, err)
	}
	return rec
}
