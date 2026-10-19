package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// command is commandContext with no deadline, which is all these tests need.
func (s sandboxSpec) command(dir, name string, args ...string) *exec.Cmd {
	return s.commandContext(context.Background(), dir, name, args...)
}

func TestPublishRebuildsFromTheTagCommitAndSaysSo(t *testing.T) {
	d := newDraft(t, defaultOpts())
	var out bytes.Buffer
	if err := d.publish("v1.3.0", &out); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !strings.Contains(out.String(), "rebuilt binaries match the draft byte for byte") {
		t.Errorf("output does not report the rebuild:\n%s", out.String())
	}
}

func TestPublishRefusesWhenTheRebuildDiffersFromTheDraft(t *testing.T) {
	o := defaultOpts()
	o.buildDiffers = true
	d := newDraft(t, o)
	err := d.publish("v1.3.0", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "rebuilt from commit") {
		t.Fatalf("publish = %v, want a rebuild mismatch", err)
	}
	if d.exists("uploaded.sig") || d.exists("published") {
		t.Fatal("publish changed the release although the rebuild differed")
	}
}

// The operator's own tree embeds the right key, but the tag's commit swapped it.
func TestPublishRefusesWhenTheTagsCommitEmbedsAnotherKey(t *testing.T) {
	o := defaultOpts()
	o.commitKeyDiffers = true
	d := newDraft(t, o)
	err := d.publish("v1.3.0", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "embedded in the tag's own commit") {
		t.Fatalf("publish = %v, want a refusal naming the tag's commit", err)
	}
	if d.exists("uploaded.sig") || d.exists("published") {
		t.Fatal("publish changed the release")
	}
}

func TestPublishRefusesWhenTheCommitDeclaresAnotherVersion(t *testing.T) {
	o := defaultOpts()
	o.commitVersion = "1.2.0"
	d := newDraft(t, o)
	err := d.publish("v1.3.0", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "declares version") {
		t.Fatalf("publish = %v, want a version refusal", err)
	}
	if d.exists("uploaded.sig") {
		t.Fatal("publish uploaded a signature")
	}
}

func TestPublishRefusesWhenTheTagCommitCannotBeResolved(t *testing.T) {
	d := newDraft(t, defaultOpts())
	if err := os.WriteFile(filepath.Join(d.state, "commit"), []byte("not-a-commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.publish("v1.3.0", &bytes.Buffer{}); err == nil {
		t.Fatal("an unresolvable tag commit must stop publish")
	}
	if d.exists("uploaded.sig") {
		t.Fatal("publish uploaded a signature")
	}
}

// If the draft is edited after it was verified, nothing becomes public.
func TestPublishRefusesToPublishADraftChangedAfterTheChecks(t *testing.T) {
	for name, mutate := range map[string]func(*draftOpts){
		"SHA256SUMS changed": func(o *draftOpts) { o.lateSums = true },
		"a binary changed":   func(o *draftOpts) { o.lateBinary = true },
	} {
		t.Run(name, func(t *testing.T) {
			o := defaultOpts()
			mutate(&o)
			d := newDraft(t, o)
			err := d.publish("v1.3.0", &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "changed in the draft") {
				t.Fatalf("publish = %v, want a refusal for a changed draft", err)
			}
			if d.exists("published") {
				t.Fatal("the draft was published although it changed")
			}
			if strings.Contains(d.log(), "--draft=false") {
				t.Fatal("publish tried to un-draft a changed release")
			}
			if !strings.Contains(err.Error(), "gh release delete-asset v1.3.0 SHA256SUMS.sig") {
				t.Errorf("the error does not say how to remove the uploaded signature: %v", err)
			}
		})
	}
}

func TestLoadSigningKeyOnWindowsWarnsInsteadOfRefusing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.hex")
	if err := os.WriteFile(path, []byte(strings.Repeat("ab", 32)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := hostOS
	t.Cleanup(func() { hostOS = old })

	hostOS = "windows"
	var warn bytes.Buffer
	if _, err := loadSigningKey(path, &warn); err != nil {
		t.Fatalf("on Windows the mode must not refuse: %v", err)
	}
	if !strings.Contains(warn.String(), "warning") {
		t.Errorf("no warning was printed: %q", warn.String())
	}

	hostOS = "linux"
	if _, err := loadSigningKey(path, io.Discard); err == nil {
		t.Fatal("on Linux an open key file must still be refused")
	}
}

// The rebuild must use the flags build.sh uses, or no hash will ever match.
func TestRebuildFlagsMatchBuildScript(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CGO_ENABLED=0", "-trimpath", "-buildvcs=false", "-tags minified", "-X 'modelsdb/internal/shared.Version=$APP_VERSION'"} {
		if !strings.Contains(string(script), want) {
			t.Errorf("build.sh lacks %q", want)
		}
	}
	args := strings.Join(buildArgs("1.2.3", "out"), " ")
	for _, want := range []string{"-trimpath", "-buildvcs=false", "-tags minified", "-X 'modelsdb/internal/shared.Version=1.2.3'", "./cmd/modelsdb"} {
		if !strings.Contains(args, want) {
			t.Errorf("build arguments lack %q: %s", want, args)
		}
	}
	env := strings.Join(buildEnv("windows"), " ")
	for _, want := range []string{"CGO_ENABLED=0", "GOOS=windows", "GOARCH=amd64", "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local"} {
		if !strings.Contains(env, want) {
			t.Errorf("build environment lacks %q: %s", want, env)
		}
	}
}

func TestEmbeddedKeyAtReadsExactlyOneConstant(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "selfupdate"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "internal", "selfupdate", "pubkey.go")
	os.WriteFile(path, []byte("package selfupdate\n\nconst releasePubKeyB64 = \"AAAA\"\n"), 0o644)
	if got, err := embeddedKeyAt(dir); err != nil || got != "AAAA" {
		t.Fatalf("embeddedKeyAt = %q, %v", got, err)
	}
	os.WriteFile(path, []byte("package selfupdate\n\nconst releasePubKeyB64 = \"AAAA\"\nconst releasePubKeyB64 = \"BBBB\"\n"), 0o644)
	if _, err := embeddedKeyAt(dir); err == nil {
		t.Fatal("two constants must be refused")
	}
	os.WriteFile(path, []byte("package selfupdate\n"), 0o644)
	if _, err := embeddedKeyAt(dir); err == nil {
		t.Fatal("no constant must be refused")
	}
}

func TestPublishRequiresTheCommitFlagBeforeAnyCall(t *testing.T) {
	d := newDraft(t, defaultOpts())
	for _, commit := range [][]string{nil, {"-commit", "abc"}, {"-commit", strings.ToUpper(d.commit)}} {
		args := append([]string{"publish", "-tag", "v1.3.0", "-key", d.keyFile, "-repo", "owner/repo"}, commit...)
		if err := run(args, &bytes.Buffer{}, noEnv); err == nil {
			t.Errorf("publish accepted -commit %v", commit)
		}
	}
	if d.log() != "" {
		t.Fatalf("gh was called without a valid -commit:\n%s", d.log())
	}
}

// A tag moved to another commit is refused before anything is fetched or built.
func TestPublishRefusesWhenTheTagPointsAtAnotherCommit(t *testing.T) {
	d := newDraft(t, defaultOpts())
	fetched, built := false, false
	oldSrc, oldBuild := sourceURL, buildBinaries
	t.Cleanup(func() { sourceURL, buildBinaries = oldSrc, oldBuild })
	sourceURL = func(r string) string { fetched = true; return oldSrc(r) }
	buildBinaries = func(buildJob) error { built = true; return nil }

	err := run([]string{"publish", "-tag", "v1.3.0", "-commit", strings.Repeat("a", 40), "-key", d.keyFile, "-repo", "owner/repo"}, &bytes.Buffer{}, noEnv)
	if err == nil || !strings.Contains(err.Error(), "not the "+strings.Repeat("a", 40)) {
		t.Fatalf("publish = %v, want a refusal naming both commits", err)
	}
	if fetched || built {
		t.Fatalf("source was fetched (%v) or built (%v) for the wrong commit", fetched, built)
	}
	if d.exists("uploaded.sig") || d.exists("published") {
		t.Fatal("publish changed the release")
	}
}

func TestPublishResolvesTheTagThroughRefsTags(t *testing.T) {
	d := newDraft(t, defaultOpts())
	if err := d.publish("v1.3.0", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.log(), "api repos/owner/repo/commits/refs/tags/v1.3.0") {
		t.Fatalf("the tag was not resolved as refs/tags/<tag>:\n%s", d.log())
	}
}

func TestPublishRefusesWithoutBubblewrapBeforeAnyCall(t *testing.T) {
	d := newDraft(t, defaultOpts())
	sandboxBinary = func() string { return "" }
	err := d.publish("v1.3.0", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "bubblewrap") {
		t.Fatalf("publish = %v, want a refusal naming bubblewrap", err)
	}
	if d.log() != "" {
		t.Fatalf("gh was called although there is no sandbox:\n%s", d.log())
	}
	// The real build function refuses on its own too, never running unsandboxed.
	if err := buildReleaseBinaries(buildJob{Src: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "bubblewrap") {
		t.Fatalf("buildReleaseBinaries = %v, want a refusal naming bubblewrap", err)
	}
}

// A VERSION that is a link must not be followed, and what it points at must not
// appear anywhere in the error.
func TestPublishRefusesASymlinkedVersionAndPrintsNothingOfIt(t *testing.T) {
	target := filepath.Join(t.TempDir(), "dummy-secret")
	if err := os.WriteFile(target, []byte("DUMMY-SECRET-0123456789\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := defaultOpts()
	o.versionLink = target
	d := newDraft(t, o)
	var out bytes.Buffer
	err := d.publish("v1.3.0", &out)
	if err == nil {
		t.Fatal("a symlinked VERSION was accepted")
	}
	if strings.Contains(err.Error(), "DUMMY-SECRET") || strings.Contains(out.String(), "DUMMY-SECRET") {
		t.Fatalf("the linked file's content reached the output: %v / %s", err, out.String())
	}
	if d.exists("uploaded.sig") {
		t.Fatal("publish uploaded a signature")
	}
}

func TestVersionAtAcceptsOnlyXYZAndNeverEchoesTheFile(t *testing.T) {
	write := func(content string) string {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "VERSION"), []byte(content), 0o644)
		return dir
	}
	if v, err := versionAt(write("10.11.0\n")); err != nil || v != "10.11.0" {
		t.Fatalf("versionAt = %q, %v", v, err)
	}
	for _, bad := range []string{"", "1.2", "01.2.3", "1.2.3-rc1", "v1.2.3", "DUMMY-SECRET-0123", "1.2.3\nDUMMY-SECRET", strings.Repeat("1", 200)} {
		_, err := versionAt(write(bad))
		if err == nil {
			t.Errorf("VERSION %q was accepted", bad)
		} else if strings.Contains(err.Error(), "DUMMY") {
			t.Errorf("the error echoes the file: %v", err)
		}
	}
}

func TestPlainFileInRefusesLinksAnywhereOnThePath(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "pubkey.go"), []byte("x"), 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	// internal/selfupdate is a link to a folder elsewhere.
	if err := os.Symlink(outside, filepath.Join(dir, "internal", "selfupdate")); err != nil {
		t.Fatal(err)
	}
	if _, err := embeddedKeyAt(dir); err == nil {
		t.Fatal("a pubkey.go reached through a linked folder was read")
	}
	// pubkey.go itself is a link.
	dir2 := t.TempDir()
	os.MkdirAll(filepath.Join(dir2, "internal", "selfupdate"), 0o755)
	if err := os.Symlink(filepath.Join(outside, "pubkey.go"), filepath.Join(dir2, "internal", "selfupdate", "pubkey.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := embeddedKeyAt(dir2); err == nil {
		t.Fatal("a linked pubkey.go was read")
	}
}

// The sandboxed child must not be able to read a file at the key's path, whether
// the file is outside the home folder (the temp folder here), reached through a
// link, or inside a folder the sandbox does bind. The files hold a dummy value;
// the real seed is never used.
func TestSandboxedChildCannotReadTheKeyFile(t *testing.T) {
	bwrap := "/usr/bin/bwrap"
	if _, err := os.Stat(bwrap); err != nil {
		t.Skip("bubblewrap is not installed")
	}
	self := buildStaticHelper(t)
	const dummy = "DUMMY-KEY-FOR-THE-SANDBOX-TEST\n"
	key := filepath.Join(t.TempDir(), "seed.hex") // outside the home folder
	src, out, cache := t.TempDir(), t.TempDir(), t.TempDir()
	inside := filepath.Join(src, "seed.hex") // inside a folder the sandbox binds
	for _, p := range []string{key, inside} {
		if err := os.WriteFile(p, []byte(dummy), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "link.hex")
	if err := os.Symlink(key, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRET_IN_PARENT_ENV", "DUMMY-ENV-VALUE")
	old := sandboxBinary
	sandboxBinary = func() string { return bwrap }
	t.Cleanup(func() { sandboxBinary = old })

	// Positive control: a bound folder's file is readable when nothing hides it,
	// so the empty reads below are the overlay's doing and not a broken helper.
	control, err := sandboxFor(buildJob{Src: src, OutDir: out}, testGoRoot(t), cache, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	control.ReadOnly = append(control.ReadOnly, self)
	control.Env = append(control.Env, "SANDBOX_READ="+inside)
	ccmd := control.command(src, self)
	var cbuf bytes.Buffer
	ccmd.Stdout, ccmd.Stderr = &cbuf, &cbuf
	if err := ccmd.Run(); err != nil || !strings.Contains(cbuf.String(), "DUMMY-KEY") {
		t.Fatalf("control run did not read the unhidden file: %v\n%s", err, cbuf.String())
	}

	for _, keyPath := range []string{key, link} {
		box, err := sandboxFor(buildJob{Src: src, OutDir: out, KeyPath: keyPath}, testGoRoot(t), cache, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		box.ReadOnly = append(box.ReadOnly, self) // the helper is the child
		box.Hidden = append(box.Hidden, inside)
		for _, target := range []string{key, link, inside} {
			b := box
			b.Env = append(append([]string{}, box.Env...), "SANDBOX_READ="+target)
			cmd := b.command(src, self)
			var buf bytes.Buffer
			cmd.Stdout, cmd.Stderr = &buf, &buf
			if err := cmd.Run(); err != nil {
				t.Fatalf("sandboxed child reading %s: %v\n%s", target, err, buf.String())
			}
			got := buf.String()
			if strings.Contains(got, "DUMMY-KEY") {
				t.Fatalf("the sandboxed child read the key through %s:\n%s", target, got)
			}
			if !strings.Contains(got, "HELPER-RESULT empty-or-missing") {
				t.Fatalf("the child did not report on %s:\n%s", target, got)
			}
			if strings.Contains(got, "DUMMY-ENV-VALUE") || !strings.Contains(got, "HELPER-ENV clean") {
				t.Fatalf("the parent's environment reached the child:\n%s", got)
			}
		}
	}
}

// helperSource is the child of the sandbox test: it reports whether it can read
// the file named by SANDBOX_READ and whether the parent's secret variable is set.
const helperSource = `package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile(os.Getenv("SANDBOX_READ"))
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		fmt.Println("HELPER-RESULT empty-or-missing")
	} else {
		fmt.Printf("HELPER-RESULT readable %s\n", data)
	}
	if os.Getenv("SECRET_IN_PARENT_ENV") == "" {
		fmt.Println("HELPER-ENV clean")
	}
}
`

// buildStaticHelper compiles helperSource without cgo, so it runs in a sandbox
// that holds no C library.
func buildStaticHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(helperSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module helper\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "helper")
	cmd := exec.Command(goBinary(), "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the helper: %v\n%s", err, out)
	}
	return bin
}

func testGoRoot(t *testing.T) string {
	t.Helper()
	root, err := goRoot()
	if err != nil {
		t.Skipf("no usable Go root: %v", err)
	}
	return root
}
