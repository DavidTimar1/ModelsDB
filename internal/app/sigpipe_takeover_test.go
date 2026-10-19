package app

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestBareTakeoverSurvivesAnEarlyClosedOutputPipe reproduces the incident of
// 2026-08-27:
// `~/.local/bin/modelsdb 2>&1 | head -1` SIGKILLed the running instance, then
// died itself writing the very next log line - "Stopped previous instance" -
// because `head` had already closed the pipe and Go's default SIGPIPE action
// is to terminate. The incumbent was already dead, so nothing was left
// serving. This test drives two REAL bare, non-interactive invocations (stdin
// is the null device, same as the incident's piped shell) against a disposable
// sandbox, closes the second one's combined stdout+stderr pipe after one line
// - exactly what `| head -1` does - and asserts a server answers afterwards.
func TestBareTakeoverSurvivesAnEarlyClosedOutputPipe(t *testing.T) {
	s := newSandbox(t)
	bin := modelsdbBinary(t)

	first := exec.Command(bin, bareArgs(s)...)
	first.Env = childEnv(s.dir)
	first.Stdin = nil // non-interactive: no controlling terminal, like a piped or backgrounded launch
	first.Stdout, first.Stderr = io.Discard, io.Discard
	if err := first.Start(); err != nil {
		t.Fatalf("cannot start the first instance: %v", err)
	}
	t.Cleanup(func() { endProcess(first) })
	if !waitHealthy(s.port, first.Process.Pid, 30*time.Second) {
		t.Fatalf("the first instance on port %d never became healthy", s.port)
	}

	second := exec.Command(bin, bareArgs(s)...)
	second.Env = childEnv(s.dir)
	second.Stdin = nil
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create the output pipe: %v", err)
	}
	second.Stdout = pw
	second.Stderr = pw
	if err := second.Start(); err != nil {
		pr.Close()
		pw.Close()
		t.Fatalf("cannot start the second instance: %v", err)
	}
	pw.Close() // the child holds its own duplicate; this process needs no write end
	t.Cleanup(func() { endProcess(second) })

	// Read exactly one line, then close the read end - `head -1`'s exact effect.
	// Whatever the second process writes next lands on a pipe with no reader.
	lineRead := make(chan struct{})
	go func() {
		bufio.NewReader(pr).ReadString('\n')
		close(lineRead)
		pr.Close()
	}()
	select {
	case <-lineRead:
	case <-time.After(10 * time.Second):
		t.Fatalf("the second instance never wrote a line to take over on")
	}

	// The takeover itself (SIGKILLing the first instance) is not what this test
	// checks - KillPreviousInstance is deliberate and unconditional. What matters
	// is that SOMETHING answers afterwards: either the second instance survived
	// its own log write to the now-closed pipe, or - at minimum - the process
	// that held the port did not vanish leaving nothing serving.
	if !waitHealthy(s.port, second.Process.Pid, 30*time.Second) {
		t.Fatalf("nothing is serving on port %d after the takeover: the second instance was killed writing to the closed output pipe, exactly the 2026-08-27 incident", s.port)
	}
}

// bareArgs are the global flags that pin a bare (no-subcommand) invocation to
// the sandbox, mirroring sandbox.args but with no leading command token - a
// bare invocation is exactly what the incident used.
func bareArgs(s sandbox) []string {
	return s.args("serve")[1:]
}
