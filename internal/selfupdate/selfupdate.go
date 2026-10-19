// Package selfupdate replaces a shipped ModelsDB binary with a newer signed
// release, in place, and re-launches the process.
//
// The flow is deliberately fail-closed. The signed SHA256SUMS and its ed25519
// signature are fetched first and checked with the embedded public key; only
// then is the binary downloaded, and its SHA-256 matched against its line in the
// signed file. The signed file also carries the release version, and the update
// is refused unless that version is newer than the running one and equals the
// version the update check announced: the decision comes from signed content,
// never from the release tag, so an old signed release cannot be replayed under
// a newer tag. If anything is missing or invalid the running binary is left
// untouched. There is no unsigned fallback path.
//
// Releases are signed on the maintainer's own machine (cmd/sign publish), never
// in CI, so no signing key exists in GitHub.
//
// Platform split: on unix the running executable can be renamed/overwritten
// while it runs, so the new binary is renamed over the old one and the process
// re-execs itself (execve). On Windows a running .exe cannot be overwritten, so
// the old file is renamed aside to "<exe>.old", the new file takes its place,
// and a fresh process is started while this one exits. The leftover .old is
// removed on the next startup.
//
// Every GitHub/download request goes out through shared.NewHTTPClient, so it
// carries the one plain browser User-Agent and no app- or owner-identifying
// header, matching the update checker.
package selfupdate

import (
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"net/http"
	"os"
	"runtime"
	"strings"
)

// Progress reports a coarse stage of Stage back to the caller ("downloading" or
// "verifying") so the UI can reflect it. It may be nil.
type Progress func(phase string)

// Progress phase names passed to a Progress callback.
const (
	PhaseDownloading = "downloading"
	PhaseVerifying   = "verifying"
)

// downloadBaseURL is the GitHub "latest release" download base for the
// configured upstream repo. Assets live at "<base>/<name>". "latest" never
// resolves to a draft, so a release is invisible here until it is published.
func downloadBaseURL() string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/latest/download",
		paths.ActiveUpstream.Owner, paths.ActiveUpstream.Repo)
}

// executablePath is the path of the running binary, falling back to argv[0].
func executablePath() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return os.Args[0]
}

// stager holds everything one staging run depends on, so a test can point it at
// a local server and a throwaway executable. Stage fills it from the real world.
type stager struct {
	base      string       // release download base URL; assets live at base/<name>
	pubKeyB64 string       // embedded release public key
	installed string       // running version
	asset     string       // asset name for this platform
	exe       string       // path of the running executable
	client    *http.Client // used for every download
}

// Stage downloads the latest release binary for this platform, verifies it, and
// swaps it into place next to the running executable so a Restart applies it.
//
// It returns (true, nil) only when the new binary is in place. Any failure -
// unsupported platform, dev build, missing signing key, download error, bad
// signature, a signed version that is not newer or that differs from
// latestVersion, or hash mismatch - returns (false, err) and leaves the running
// binary untouched. progress may be nil.
func Stage(latestVersion string, progress Progress) (bool, error) {
	if shared.IsDevVersion() {
		return false, errors.New("self-update is disabled for development builds")
	}
	if !Supported() {
		return false, fmt.Errorf("self-update is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if !pubKeyConfigured() {
		return false, errSigningKeyMissing
	}
	name, _ := assetName()
	s := stager{
		base:      downloadBaseURL(),
		pubKeyB64: releasePubKeyB64,
		installed: shared.Version,
		asset:     name,
		exe:       executablePath(),
		client:    shared.NewHTTPClient(downloadTimeout),
	}
	return s.run(latestVersion, progress)
}

// run performs the staging: signed checksums first, version checks on the signed
// content, then the binary, then the swap.
func (s stager) run(latestVersion string, progress Progress) (bool, error) {
	if strings.TrimSpace(s.pubKeyB64) == "" {
		return false, errSigningKeyMissing
	}
	report := func(phase string) {
		if progress != nil {
			progress(phase)
		}
	}

	// 1. Fetch and verify the signed checksums BEFORE the binary is downloaded.
	report(PhaseDownloading)
	sigRaw, err := httpGetBytes(s.client, s.base+"/"+SigFile, maxSigBytes)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", SigFile, err)
	}
	sumsRaw, err := httpGetBytes(s.client, s.base+"/"+SumsFile, maxSumsBytes)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", SumsFile, err)
	}
	report(PhaseVerifying)
	sig, err := DecodeSignature(sigRaw)
	if err != nil {
		return false, err
	}
	if err := VerifySumsWithKey(s.pubKeyB64, sumsRaw, sig); err != nil {
		return false, err
	}
	sums, err := ParseSums(sumsRaw)
	if err != nil {
		return false, err
	}

	// 2. The signed version decides, never the tag: it must be newer than the
	// running version and must be the release the update check announced.
	if err := CheckNewer(s.installed, sums.Version); err != nil {
		return false, err
	}
	if want := strings.TrimPrefix(strings.TrimSpace(latestVersion), "v"); sums.Version != want {
		return false, fmt.Errorf("signed release version %s differs from the announced version %q; refusing", sums.Version, want)
	}
	if _, ok := sums.Hash(s.asset); !ok {
		return false, fmt.Errorf("SHA256SUMS has no entry for %q", s.asset)
	}

	// 3. Only now download the binary and match its hash.
	report(PhaseDownloading)
	asset, err := httpGetBytes(s.client, s.base+"/"+s.asset, maxBinaryBytes)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", s.asset, err)
	}
	report(PhaseVerifying)
	if err := sums.CheckAsset(s.asset, asset); err != nil {
		return false, err
	}

	// 4. Write the verified bytes to a temp file in the SAME directory as the
	// running exe (same filesystem, so the swap rename is atomic), then apply.
	// A partial temp file must never linger, so remove it on either error path.
	tmp, err := writeTempBeside(s.exe, asset)
	if err != nil {
		return false, fmt.Errorf("write temp binary: %w", err)
	}
	if err := apply(s.exe, tmp); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("apply update: %w", err)
	}

	log.Printf("Self-update staged: v%s (signed) replaces v%s at %s; restart to apply", sums.Version, s.installed, s.exe)
	return true, nil
}
