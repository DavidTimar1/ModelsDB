package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"modelsdb/internal/paths"
	"modelsdb/internal/selfupdate"
)

// repoEnv names the environment variable that selects the GitHub OWNER/REPO the
// draft lives in when -repo is not given.
const repoEnv = "MODELSDB_RELEASE_REPO"

// ghPaths are the only places the GitHub CLI is looked for. A program that signs
// a release is never pointed at gh through PATH, since PATH can name a folder
// another user can write to.
var ghPaths = []string{"/usr/bin/gh", "/usr/local/bin/gh", "/bin/gh", "/snap/bin/gh"}

// ghBinary finds the GitHub CLI. It is a variable so a test can point it at a fake.
var ghBinary = func() string {
	for _, p := range ghPaths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

var (
	publishTagRe = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	repoRe       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// draftRelease is the part of `gh release view --json` that publish reads.
type draftRelease struct {
	TagName string `json:"tagName"`
	IsDraft bool   `json:"isDraft"`
	Assets  []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func (d draftRelease) hasAsset(name string) bool {
	for _, a := range d.Assets {
		if a.Name == name {
			return true
		}
	}
	return false
}

// ghHost runs the GitHub CLI as the operator who runs sign, with their own
// login. No token is read, passed or printed here.
type ghHost struct{ bin string }

func (g ghHost) run(stdout io.Writer, args ...string) error {
	cmd := exec.Command(g.bin, args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1")
	cmd.Stdout, cmd.Stderr = stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh %s %s: %w", args[0], args[1], err)
	}
	return nil
}

func (g ghHost) view(repo, tag string) (draftRelease, error) {
	var out bytes.Buffer
	if err := g.run(&out, "release", "view", tag, "--repo", repo, "--json", "tagName,isDraft,assets"); err != nil {
		return draftRelease{}, fmt.Errorf("%w (is gh logged in as you, with access to %s?)", err, repo)
	}
	var d draftRelease
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		return draftRelease{}, fmt.Errorf("gh printed an unreadable release description: %w", err)
	}
	return d, nil
}

// commitOf resolves the full commit the tag points at, through refs/tags/ so a
// branch of the same name can never be taken for the tag. The rebuild that gates
// signing is made from exactly this commit, so a failure to resolve it is fatal.
func (g ghHost) commitOf(repo, tag string) (string, error) {
	var out bytes.Buffer
	if err := g.run(&out, "api", fmt.Sprintf("repos/%s/commits/refs/tags/%s", repo, tag), "--jq", ".sha"); err != nil {
		return "", fmt.Errorf("resolve the commit of %s: %w", tag, err)
	}
	sha := strings.TrimSpace(out.String())
	if !commitRe.MatchString(sha) {
		return "", fmt.Errorf("the commit of %s is %q, not a full commit hash", tag, sha)
	}
	return sha, nil
}

// fetchDraftFile downloads one named asset of the draft into dir and returns its bytes.
func (g ghHost) fetchDraftFile(repo, tag, dir, name string) ([]byte, error) {
	if err := g.run(io.Discard, "release", "download", tag, "--repo", repo, "--dir", dir, "--pattern", name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("downloaded %s: %w", name, err)
	}
	return data, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// cmdPublish completes a release on the operator's machine. It takes the draft
// CI built, recomputes the hash of every binary in it against SHA256SUMS, signs
// SHA256SUMS locally, uploads the signature while the release is still a draft
// and only then publishes it. The key never leaves this machine and is never
// printed.
func cmdPublish(args []string, stdout io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	tag := fs.String("tag", "", "the release tag, vX.Y.Z")
	keyPath := fs.String("key", "", "file holding the hex signing seed (kept outside every repository)")
	commitFlag := fs.String("commit", "", "the 40-hex commit you released, from `git rev-parse vX.Y.Z^{commit}` in your own clone")
	repoFlag := fs.String("repo", "", "GitHub OWNER/REPO that holds the draft (default: "+repoEnv+", else the upstream repo this source tree is built for)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !publishTagRe.MatchString(*tag) {
		return fmt.Errorf("-tag must look like v1.2.3, got %q", *tag)
	}
	if !commitRe.MatchString(*commitFlag) {
		return errors.New("-commit is required: the full 40-character commit hash you released (git rev-parse <tag>^{commit} in your own clone)")
	}
	if *keyPath == "" {
		return errors.New("-key is required: the file holding the signing seed")
	}
	repo := strings.TrimSpace(*repoFlag)
	if repo == "" {
		repo = strings.TrimSpace(getenv(repoEnv))
	}
	if repo == "" {
		repo = paths.ActiveUpstream.Owner + "/" + paths.ActiveUpstream.Repo
	}
	if !repoRe.MatchString(repo) {
		return fmt.Errorf("repository %q is not of the form OWNER/REPO", repo)
	}
	gh := ghBinary()
	if gh == "" {
		return fmt.Errorf("the GitHub CLI (gh) was not found at %s", strings.Join(ghPaths, ", "))
	}

	if sandboxBinary() == "" {
		return sandboxMissing()
	}

	// Everything that needs no network comes first: the key must be the embedded one.
	priv, err := loadSigningKey(*keyPath, os.Stderr)
	if err != nil {
		return err
	}
	if err := checkKeyMatchesEmbedded(priv); err != nil {
		return err
	}

	host := ghHost{bin: gh}
	rel, err := host.view(repo, *tag)
	if err != nil {
		return err
	}
	switch {
	case !rel.IsDraft:
		return fmt.Errorf("release %s of %s is not a draft; refusing to sign a release that is already public", *tag, repo)
	case rel.hasAsset(selfupdate.SigFile):
		return fmt.Errorf("release %s already has %s; refusing to sign it twice", *tag, selfupdate.SigFile)
	case !rel.hasAsset(selfupdate.SumsFile):
		return fmt.Errorf("release %s has no %s to sign", *tag, selfupdate.SumsFile)
	}

	work, err := os.MkdirTemp("", "sign-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err := host.run(io.Discard, "release", "download", *tag, "--repo", repo, "--dir", work, "--pattern", selfupdate.SumsFile); err != nil {
		return err
	}
	sumsRaw, err := os.ReadFile(filepath.Join(work, selfupdate.SumsFile))
	if err != nil {
		return err
	}
	sums, err := selfupdate.ParseSums(sumsRaw)
	if err != nil {
		return err
	}
	if want := strings.TrimPrefix(*tag, "v"); sums.Version != want {
		return fmt.Errorf("%s names version %s but the tag is %s; refusing to sign", selfupdate.SumsFile, sums.Version, *tag)
	}
	for _, name := range sums.Names() {
		if !rel.hasAsset(name) {
			return fmt.Errorf("%s lists %s but the draft has no such asset; refusing to sign", selfupdate.SumsFile, name)
		}
	}

	// Recompute every binary's hash from the draft itself, so a hand-edited
	// SHA256SUMS cannot get a different binary signed.
	commit, err := host.commitOf(repo, *tag)
	if err != nil {
		return err
	}
	if commit != *commitFlag {
		return fmt.Errorf("tag %s of %s points at commit %s, not the %s you gave with -commit; nothing was fetched or built. "+
			"Someone may have moved the tag: look at what changed before going on", *tag, repo, commit, *commitFlag)
	}
	fmt.Fprintf(stdout, "release %s of %s, version %s, tag commit %s\n", *tag, repo, sums.Version, commit)
	draftHashes := map[string]string{}
	for _, name := range sums.Names() {
		data, err := host.fetchDraftFile(repo, *tag, work, name)
		if err != nil {
			return err
		}
		got := sha256Hex(data)
		want, _ := sums.Hash(name)
		if got != want {
			return fmt.Errorf("%s in the draft hashes to %s but %s lists %s; refusing to sign", name, got, selfupdate.SumsFile, want)
		}
		draftHashes[name] = got
		fmt.Fprintf(stdout, "  %s  %s\n", got, name)
	}

	// CI built these binaries, so nothing so far says they come from the source.
	// Rebuild them here from the tag's own commit and require identical bytes.
	if err := verifyRebuild(stdout, repo, commit, *keyPath, sums, priv, work); err != nil {
		return err
	}

	sig := ed25519.Sign(priv, sumsRaw)
	if err := selfupdate.VerifySumsWithKey(embeddedPubKey(), sumsRaw, sig); err != nil {
		return fmt.Errorf("the new signature does not verify: %w", err)
	}
	sigPath := filepath.Join(work, selfupdate.SigFile)
	if err := os.WriteFile(sigPath, selfupdate.EncodeSignature(sig), 0o600); err != nil {
		return err
	}
	if err := host.run(io.Discard, "release", "upload", *tag, sigPath, "--repo", repo); err != nil {
		return err
	}

	// The draft may have been edited while the rebuild ran. Compare it with what
	// was verified just before it becomes public.
	if err := recheckDraft(host, repo, *tag, work, sumsRaw, draftHashes); err != nil {
		return fmt.Errorf("%w (the signature is already uploaded to the draft; remove it with: gh release delete-asset %s %s --repo %s)", err, *tag, selfupdate.SigFile, repo)
	}
	if err := host.run(io.Discard, "release", "edit", *tag, "--repo", repo, "--draft=false"); err != nil {
		return fmt.Errorf("%w (the signature is uploaded; finish with: gh release edit %s --repo %s --draft=false)", err, *tag, repo)
	}
	fmt.Fprintf(stdout, "signed and published release %s of %s (signature verified against the embedded public key)\n", *tag, repo)
	return nil
}
