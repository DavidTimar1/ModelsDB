package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanOutputDropsEscapesAndCapsLength(t *testing.T) {
	hostile := "build failed\n\x1b[31mred\x1b[0m \x1b]52;c;U0VDUkVU\a ‮evil‬\tdone\r\x00"
	got := cleanOutput(hostile)
	for _, bad := range []string{"\x1b", "\a", "\r", "\x00", "‮", "‬"} {
		if strings.Contains(got, bad) {
			t.Errorf("cleaned output still holds %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "build failed\n") || !strings.Contains(got, "\tdone") {
		t.Errorf("newline and tab must survive: %q", got)
	}
	long := cleanOutput(strings.Repeat("x", 100000))
	if len(long) > maxShownOutput+64 || !strings.Contains(long, "output cut") {
		t.Errorf("long output was not cut: %d bytes", len(long))
	}
}

func needSh(t *testing.T) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	return sh
}

func TestRunLimitedCleansWhatTheChildPrints(t *testing.T) {
	sh := needSh(t)
	out, err := runLimited(time.Minute, t.TempDir(), func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, sh, "-c", `printf 'ok \033]52;c;QUJD\007 end'; exit 3`)
	})
	if err == nil {
		t.Fatal("the child's failure was lost")
	}
	if strings.ContainsAny(out, "\x1b\a") || !strings.Contains(out, "ok") {
		t.Errorf("output not cleaned: %q", out)
	}
}

func TestRunLimitedStopsACommandThatRunsTooLong(t *testing.T) {
	sh := needSh(t)
	start := time.Now()
	_, err := runLimited(300*time.Millisecond, t.TempDir(), func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, sh, "-c", "exec sleep 30")
	})
	if !errors.Is(err, errTimedOut) {
		t.Fatalf("err = %v, want the fixed time-limit refusal", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("the command was not stopped promptly: %v", time.Since(start))
	}
}

func TestRunLimitedStopsACommandThatFillsTheWorkFolder(t *testing.T) {
	sh := needSh(t)
	oldMax, oldEvery := maxWorkBytes, watchEvery
	maxWorkBytes, watchEvery = 1<<20, 50*time.Millisecond
	t.Cleanup(func() { maxWorkBytes, watchEvery = oldMax, oldEvery })
	work := t.TempDir()
	_, err := runLimited(time.Minute, work, func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, sh, "-c", "head -c 3000000 /dev/zero > '"+filepath.Join(work, "big")+"'; exec sleep 30")
	})
	if !errors.Is(err, errTooBig) {
		t.Fatalf("err = %v, want the fixed size-limit refusal", err)
	}
}

func TestGoRootMustBeAbsoluteAndHoldBinGo(t *testing.T) {
	old := goBinary
	t.Cleanup(func() { goBinary = old })

	// What an empty runtime.GOROOT() produces.
	goBinary = func() string { return filepath.Join("", "bin", "go") }
	if _, err := goRoot(); err == nil {
		t.Fatal("a relative Go root was accepted")
	}
	goBinary = func() string { return filepath.Join(t.TempDir(), "bin", "go") }
	if _, err := goRoot(); err == nil {
		t.Fatal("a Go root without bin/go was accepted")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "bin", "go"), []byte("x"), 0o755)
	goBinary = func() string { return filepath.Join(root, "bin", "go") }
	if got, err := goRoot(); err != nil || got != root {
		t.Fatalf("goRoot = %q, %v", got, err)
	}
	// And the build itself refuses before binding anything.
	goBinary = func() string { return "bin/go" }
	if err := buildReleaseBinaries(buildJob{Src: t.TempDir()}); err == nil {
		t.Fatal("the build ran with a relative Go root")
	}
}
