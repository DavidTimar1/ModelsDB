package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"modelsdb/internal/selfupdate"
)

func noEnv(string) string { return "" }

func TestKeygenWritesTheSeedPrivatelyAndPrintsOnlyThePublicKey(t *testing.T) {
	seedPath := filepath.Join(t.TempDir(), "seed.hex")
	var out bytes.Buffer
	if err := run([]string{"keygen", "-seed-out", seedPath}, &out, noEnv); err != nil {
		t.Fatalf("keygen: %v", err)
	}
	fi, err := os.Stat(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("seed file mode = %o, want 600", fi.Mode().Perm())
	}
	raw, _ := os.ReadFile(seedPath)
	seedHex := strings.TrimSpace(string(raw))
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatalf("seed file does not hold a hex seed: %v", err)
	}
	if strings.Contains(out.String(), seedHex) {
		t.Fatal("keygen printed the seed in hex")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if strings.Contains(out.String(), base64.StdEncoding.EncodeToString(priv)) || strings.Contains(out.String(), base64.StdEncoding.EncodeToString(seed)) {
		t.Fatal("keygen printed the private key in base64")
	}
	wantPub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if !strings.Contains(out.String(), wantPub) {
		t.Fatal("keygen did not print the matching public key")
	}
}

func TestKeygenRefusesAGitWorkTree(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "deep", "er")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(repo, "seed.hex"), filepath.Join(sub, "seed.hex")} {
		var out bytes.Buffer
		err := run([]string{"keygen", "-seed-out", p}, &out, noEnv)
		if err == nil || !strings.Contains(err.Error(), "git work tree") {
			t.Errorf("keygen inside a work tree (%s) = %v, want a refusal", p, err)
		}
		if _, statErr := os.Stat(p); statErr == nil {
			t.Errorf("a seed file was written inside the work tree at %s", p)
		}
		if out.Len() != 0 {
			t.Errorf("keygen printed something before refusing: %q", out.String())
		}
	}
}

func TestKeygenRefusesToOverwrite(t *testing.T) {
	seedPath := filepath.Join(t.TempDir(), "seed.hex")
	if err := os.WriteFile(seedPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"keygen", "-seed-out", seedPath}, &out, noEnv); err == nil {
		t.Fatal("keygen overwrote an existing file")
	}
	got, _ := os.ReadFile(seedPath)
	if string(got) != "existing\n" {
		t.Fatal("the existing file was changed")
	}
}

func TestKeygenRequiresASeedPath(t *testing.T) {
	if err := run([]string{"keygen"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Fatal("keygen without -seed-out must fail")
	}
}

func TestSumsWritesAStrictFileThatTheClientParses(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "modelsdb"), filepath.Join(dir, "modelsdb.exe")
	os.WriteFile(a, []byte("linux"), 0o644)
	os.WriteFile(b, []byte("windows"), 0o644)
	out := filepath.Join(dir, "SHA256SUMS")
	if err := run([]string{"sums", "-version", "1.2.3", "-out", out, a, b}, &bytes.Buffer{}, noEnv); err != nil {
		t.Fatalf("sums: %v", err)
	}
	body, _ := os.ReadFile(out)
	if !strings.HasPrefix(string(body), "# version: 1.2.3\n") {
		t.Fatalf("no version header: %q", body)
	}
	s, err := selfupdate.ParseSums(body)
	if err != nil {
		t.Fatalf("the client refuses what sums wrote: %v", err)
	}
	if err := s.CheckAsset("modelsdb", []byte("linux")); err != nil {
		t.Error(err)
	}
	if err := s.CheckAsset("modelsdb.exe", []byte("windows")); err != nil {
		t.Error(err)
	}
}

func TestSumsRefusesABadVersionOrNoFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "modelsdb")
	os.WriteFile(a, []byte("x"), 0o644)
	if err := run([]string{"sums", "-version", "v1.2", "-out", filepath.Join(dir, "S"), a}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("a malformed version must be refused")
	}
	if err := run([]string{"sums", "-version", "1.2.3", "-out", filepath.Join(dir, "S")}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("no files must be refused")
	}
}

func TestLoadSigningKeyRefusesUnsafeOrBadFiles(t *testing.T) {
	dir := t.TempDir()
	seed := strings.Repeat("ab", ed25519.SeedSize)

	open := filepath.Join(dir, "open.hex")
	os.WriteFile(open, []byte(seed+"\n"), 0o644)
	if _, err := loadSigningKey(open, io.Discard); err == nil || strings.Contains(err.Error(), seed) {
		t.Errorf("a world-readable key file must be refused without echoing it: %v", err)
	}

	bad := filepath.Join(dir, "bad.hex")
	os.WriteFile(bad, []byte("zz-not-hex-zz\n"), 0o600)
	if _, err := loadSigningKey(bad, io.Discard); err == nil || strings.Contains(err.Error(), "zz-not-hex") {
		t.Errorf("a malformed key file must be refused without echoing it: %v", err)
	}

	good := filepath.Join(dir, "good.hex")
	os.WriteFile(good, []byte(seed+"\n"), 0o600)
	link := filepath.Join(dir, "link.hex")
	if err := os.Symlink(good, link); err == nil {
		if _, err := loadSigningKey(link, io.Discard); err == nil {
			t.Error("a symlinked key file must be refused")
		}
	}
	if _, err := loadSigningKey(good, io.Discard); err != nil {
		t.Errorf("a good key file was refused: %v", err)
	}
	if _, err := loadSigningKey(filepath.Join(dir, "missing.hex"), io.Discard); err == nil {
		t.Error("a missing key file must be refused")
	}
}

// fakeGHScript stands in for gh. It records every call in $FAKE_GH_STATE/log and
// serves the draft from files under $FAKE_GH_STATE.
const fakeGHScript = `#!/bin/sh
echo "$*" >> "$FAKE_GH_STATE/log"
case "$1 $2" in
"release view") cat "$FAKE_GH_STATE/view.json" ;;
"release download")
  dir=; pat=
  while [ $# -gt 0 ]; do
    [ "$1" = "--dir" ] && dir="$2"
    [ "$1" = "--pattern" ] && pat="$2"
    shift
  done
  src="$FAKE_GH_STATE/files/$pat"
  if [ -f "$FAKE_GH_STATE/uploaded.sig" ] && [ -f "$src.late" ]; then src="$src.late"; fi
  cp "$src" "$dir/$pat" ;;
"release upload") cp "$4" "$FAKE_GH_STATE/uploaded.sig"; [ "$(basename "$4")" = "SHA256SUMS.sig" ] || exit 7 ;;
"release edit") [ -f "$FAKE_GH_STATE/fail_edit" ] && exit 5; echo published > "$FAKE_GH_STATE/published" ;;
"api repos/"*) cat "$FAKE_GH_STATE/commit" ;;
*) exit 9 ;;
esac
`

// draft is a fake draft release plus the key that signs it.
type draft struct {
	state   string
	keyFile string
	priv    ed25519.PrivateKey
	commit  string // the commit the tag points at
}

type draftOpts struct {
	sumsVersion string
	isDraft     bool
	assets      []string // names listed as present in the release
	sumsHashOf  string   // content whose hash SHA256SUMS records for modelsdb
	exeContent  string   // content actually served for modelsdb
	otherKey    bool     // the key file holds a key that is not the embedded one
	// The tag's commit embeds a different public key than the signing key.
	commitKeyDiffers bool
	// The tag's commit declares a different VERSION than SHA256SUMS names.
	commitVersion string
	// The tag's commit stores VERSION as a symlink to this path.
	versionLink string
	// A rebuild from the tag's commit produces a different linux binary than CI's.
	buildDiffers bool
	// After the signature is uploaded the draft serves different SHA256SUMS or binary.
	lateSums, lateBinary bool
}

// makeSourceRepo creates a one-commit repository holding the files the rebuild
// reads from the tag's commit and returns its path and commit hash.
func makeSourceRepo(t *testing.T, pubB64, version, versionLink string) (string, string) {
	t.Helper()
	vcs := gitBinary()
	if vcs == "" {
		t.Skip("git is not installed in a fixed location")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "selfupdate"), 0o755); err != nil {
		t.Fatal(err)
	}
	keyGo := "package selfupdate\n\nconst releasePubKeyB64 = \"" + pubB64 + "\"\n"
	os.WriteFile(filepath.Join(dir, "internal", "selfupdate", "pubkey.go"), []byte(keyGo), 0o644)
	if versionLink != "" {
		if err := os.Symlink(versionLink, filepath.Join(dir, "VERSION")); err != nil {
			t.Fatal(err)
		}
	} else {
		os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\n"), 0o644)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "release"},
	} {
		if out, err := exec.Command(vcs, append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	out, err := exec.Command(vcs, "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return dir, strings.TrimSpace(string(out))
}

func defaultOpts() draftOpts {
	return draftOpts{
		sumsVersion: "1.3.0",
		isDraft:     true,
		assets:      []string{"modelsdb", "modelsdb.exe", "SHA256SUMS"},
		sumsHashOf:  "linux build",
		exeContent:  "linux build",
	}
}

func newDraft(t *testing.T, o draftOpts) *draft {
	t.Helper()
	state := t.TempDir()
	files := filepath.Join(state, "files")
	if err := os.MkdirAll(files, 0o755); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(state, "gh")
	if err := os.WriteFile(gh, []byte(fakeGHScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GH_STATE", state)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	embedded := base64.StdEncoding.EncodeToString(pub)
	if o.otherKey {
		embedded = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	}
	oldGH, oldKey := ghBinary, embeddedPubKey
	ghBinary = func() string { return gh }
	embeddedPubKey = func() string { return embedded }
	t.Cleanup(func() { ghBinary, embeddedPubKey = oldGH, oldKey })

	commitKey := embedded
	if o.commitKeyDiffers {
		commitKey = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	}
	commitVersion := o.sumsVersion
	if o.commitVersion != "" {
		commitVersion = o.commitVersion
	}
	srcDir, commit := makeSourceRepo(t, commitKey, commitVersion, o.versionLink)
	os.WriteFile(filepath.Join(state, "commit"), []byte(commit+"\n"), 0o644)
	oldSrc, oldBuild, oldBox := sourceURL, buildBinaries, sandboxBinary
	sourceURL = func(string) string { return srcDir }
	sandboxBinary = func() string { return "/fake/bwrap" }
	buildBinaries = func(job buildJob) error {
		if _, err := os.Stat(filepath.Join(job.Src, "VERSION")); err != nil {
			return err
		}
		out := job.OutDir
		for _, name := range job.Names {
			data, err := os.ReadFile(filepath.Join(files, name))
			if err != nil {
				return err
			}
			if o.buildDiffers && name == "modelsdb" {
				data = []byte("what this source really builds")
			}
			if err := os.WriteFile(filepath.Join(out, name), data, 0o755); err != nil {
				return err
			}
		}
		return nil
	}
	t.Cleanup(func() { sourceURL, buildBinaries, sandboxBinary = oldSrc, oldBuild, oldBox })

	sums := selfupdate.FormatSums(o.sumsVersion, map[string]string{
		"modelsdb":     sha(o.sumsHashOf),
		"modelsdb.exe": sha("windows build"),
	})
	os.WriteFile(filepath.Join(files, "SHA256SUMS"), sums, 0o644)
	os.WriteFile(filepath.Join(files, "modelsdb"), []byte(o.exeContent), 0o644)
	os.WriteFile(filepath.Join(files, "modelsdb.exe"), []byte("windows build"), 0o644)
	if o.lateSums {
		os.WriteFile(filepath.Join(files, "SHA256SUMS.late"), append(append([]byte(nil), sums...), '\n'), 0o644)
	}
	if o.lateBinary {
		os.WriteFile(filepath.Join(files, "modelsdb.late"), []byte("swapped after the checks"), 0o644)
	}

	var names []string
	for _, a := range o.assets {
		names = append(names, `{"name":"`+a+`"}`)
	}
	isDraft := "false"
	if o.isDraft {
		isDraft = "true"
	}
	view := `{"tagName":"v1.3.0","isDraft":` + isDraft + `,"assets":[` + strings.Join(names, ",") + `]}`
	os.WriteFile(filepath.Join(state, "view.json"), []byte(view), 0o644)

	keyFile := filepath.Join(t.TempDir(), "seed.hex")
	os.WriteFile(keyFile, []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600)
	return &draft{state: state, keyFile: keyFile, priv: priv, commit: commit}
}

func sha(s string) string { return sha256Hex([]byte(s)) }

func (d *draft) publish(tag string, out *bytes.Buffer) error {
	return run([]string{"publish", "-tag", tag, "-commit", d.commit, "-key", d.keyFile, "-repo", "owner/repo"}, out, noEnv)
}

func (d *draft) log() string {
	b, _ := os.ReadFile(filepath.Join(d.state, "log"))
	return string(b)
}

func (d *draft) exists(name string) bool {
	_, err := os.Stat(filepath.Join(d.state, name))
	return err == nil
}

func TestPublishSignsUploadsAndPublishesInOrder(t *testing.T) {
	d := newDraft(t, defaultOpts())
	var out bytes.Buffer
	if err := d.publish("v1.3.0", &out); err != nil {
		t.Fatalf("publish: %v", err)
	}
	sig, err := os.ReadFile(filepath.Join(d.state, "uploaded.sig"))
	if err != nil {
		t.Fatalf("no signature was uploaded: %v", err)
	}
	sums, _ := os.ReadFile(filepath.Join(d.state, "files", "SHA256SUMS"))
	rawSig, err := selfupdate.DecodeSignature(sig)
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(d.priv.Public().(ed25519.PublicKey))
	if err := selfupdate.VerifySumsWithKey(pub, sums, rawSig); err != nil {
		t.Fatalf("the uploaded signature does not verify: %v", err)
	}
	if !d.exists("published") {
		t.Fatal("the draft was not published")
	}

	log := d.log()
	order := []string{"release view v1.3.0", "release download v1.3.0", "release upload v1.3.0", "release edit v1.3.0"}
	last := -1
	for _, want := range order {
		i := strings.Index(log[last+1:], want)
		if i < 0 {
			t.Fatalf("gh was never run as %q in order; log:\n%s", want, log)
		}
		last += i + 1
	}
	if strings.Index(log, "release upload") > strings.Index(log, "release edit") {
		t.Fatal("the draft was published before the signature was uploaded")
	}
	if !strings.Contains(log, "--draft=false") {
		t.Fatal("the release was never un-drafted")
	}

	// The operator sees the version, the commit and every hash.
	commitRaw, _ := os.ReadFile(filepath.Join(d.state, "commit"))
	for _, want := range []string{"1.3.0", strings.TrimSpace(string(commitRaw)), sha("linux build"), sha("windows build")} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	seed, _ := os.ReadFile(d.keyFile)
	if strings.Contains(out.String(), strings.TrimSpace(string(seed))) || strings.Contains(log, strings.TrimSpace(string(seed))) {
		t.Fatal("the key appeared in the output or in a gh command line")
	}
}

func TestPublishRefusesAndChangesNothing(t *testing.T) {
	withSig := defaultOpts()
	withSig.assets = append([]string{"SHA256SUMS.sig"}, withSig.assets...)
	notDraft := defaultOpts()
	notDraft.isDraft = false
	noSums := defaultOpts()
	noSums.assets = []string{"modelsdb", "modelsdb.exe"}
	missing := defaultOpts()
	missing.assets = []string{"modelsdb", "SHA256SUMS"}
	badHash := defaultOpts()
	badHash.exeContent = "a different build than CI hashed"
	wrongVersion := defaultOpts()
	wrongVersion.sumsVersion = "1.2.9"

	cases := map[string]struct {
		opts draftOpts
		want string
	}{
		"not a draft":                     {notDraft, "not a draft"},
		"already signed":                  {withSig, "already has"},
		"no SHA256SUMS":                   {noSums, "no SHA256SUMS"},
		"header version differs from tag": {wrongVersion, "names version"},
		"a listed asset is missing":       {missing, "no such asset"},
		"binary does not match its hash":  {badHash, "refusing to sign"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := newDraft(t, c.opts)
			err := d.publish("v1.3.0", &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("publish = %v, want an error containing %q", err, c.want)
			}
			if d.exists("uploaded.sig") || d.exists("published") {
				t.Fatalf("publish changed the release despite refusing; log:\n%s", d.log())
			}
		})
	}
}

func TestPublishRefusesAWrongKeyBeforeAnyNetworkCall(t *testing.T) {
	o := defaultOpts()
	o.otherKey = true
	d := newDraft(t, o)
	err := d.publish("v1.3.0", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("publish = %v, want a key mismatch", err)
	}
	if d.log() != "" {
		t.Fatalf("gh was called before the key was checked:\n%s", d.log())
	}
}

func TestPublishRefusesAnOpenKeyFileBeforeAnyNetworkCall(t *testing.T) {
	d := newDraft(t, defaultOpts())
	if err := os.Chmod(d.keyFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.publish("v1.3.0", &bytes.Buffer{}); err == nil {
		t.Fatal("a world-readable key file must be refused")
	}
	if d.log() != "" {
		t.Fatalf("gh was called before the key file was checked:\n%s", d.log())
	}
}

func TestPublishRefusesABadTagOrRepo(t *testing.T) {
	d := newDraft(t, defaultOpts())
	for _, tag := range []string{"", "1.3.0", "v1.3", "v1.3.0-rc1", "--repo", "v01.3.0"} {
		if err := d.publish(tag, &bytes.Buffer{}); err == nil {
			t.Errorf("tag %q was accepted", tag)
		}
	}
	err := run([]string{"publish", "-tag", "v1.3.0", "-commit", d.commit, "-key", d.keyFile, "-repo", "--evil"}, &bytes.Buffer{}, noEnv)
	if err == nil {
		t.Error("a repo starting with a dash was accepted")
	}
	if d.log() != "" {
		t.Fatalf("gh was called for a bad argument:\n%s", d.log())
	}
}

func TestPublishTellsTheOperatorHowToFinishWhenTheLastStepFails(t *testing.T) {
	d := newDraft(t, defaultOpts())
	os.WriteFile(filepath.Join(d.state, "fail_edit"), nil, 0o644)
	err := d.publish("v1.3.0", &bytes.Buffer{})
	if err == nil {
		t.Fatal("publish should report the failed last step")
	}
	if !strings.Contains(err.Error(), "gh release edit v1.3.0 --repo owner/repo --draft=false") {
		t.Fatalf("the error does not give the finishing command: %v", err)
	}
	if !d.exists("uploaded.sig") {
		t.Fatal("the signature should already be uploaded")
	}
}
