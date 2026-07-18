// Package selfupdate replaces a shipped ModelsDB binary with a newer signed
// release, in place, and re-launches the process.
//
// The flow is deliberately fail-closed. Before anything is swapped in, the
// downloaded SHA256SUMS file is checked against an ed25519 signature made with
// the maintainer's private key (verified here with the embedded public key),
// and only then is the downloaded binary's SHA-256 matched against its line in
// that checksums file. If the signature is missing or invalid, or the hash does
// not match, the update is refused and the running binary is left untouched.
// There is no unsigned fallback path.
//
// Platform split: on unix the running executable can be renamed/overwritten
// while it runs, so the new binary is renamed over the old one and the process
// re-execs itself (execve). On Windows a running .exe cannot be overwritten, so
// the old file is renamed aside to "<exe>.old", the new file takes its place,
// and a fresh process is started while this one exits. The leftover .old is
// removed on the next startup.
//
// Privacy: every GitHub/download request carries only Go's default User-Agent,
// with no app- or owner-identifying header, matching the update checker.
package selfupdate

import (
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"os"
	"runtime"
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
// configured upstream repo. Assets live at "<base>/<name>".
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

// Stage downloads the latest release binary for this platform, verifies it, and
// swaps it into place next to the running executable so a Restart applies it.
//
// It returns (true, nil) only when the new binary is in place. Any failure -
// unsupported platform, dev build, missing signing key, download error, bad
// signature, or hash mismatch - returns (false, err) and leaves the running
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
	base := downloadBaseURL()

	report := func(phase string) {
		if progress != nil {
			progress(phase)
		}
	}

	// 1. Download the signature, the checksums, and the asset itself.
	report(PhaseDownloading)
	sigRaw, err := httpGetBytes(base + "/SHA256SUMS.sig")
	if err != nil {
		return false, fmt.Errorf("download SHA256SUMS.sig: %w", err)
	}
	sums, err := httpGetBytes(base + "/SHA256SUMS")
	if err != nil {
		return false, fmt.Errorf("download SHA256SUMS: %w", err)
	}
	asset, err := httpGetBytes(base + "/" + name)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", name, err)
	}

	// 2. Verify: signature over the SHA256SUMS bytes, THEN the asset's hash.
	report(PhaseVerifying)
	sig, err := decodeSignature(sigRaw)
	if err != nil {
		return false, err
	}
	if err := verifySums(sums, sig); err != nil {
		return false, err
	}
	if err := verifyAsset(asset, sums, name); err != nil {
		return false, err
	}

	// 3. Write the verified bytes to a temp file in the SAME directory as the
	// running exe (same filesystem, so the swap rename is atomic), then apply.
	// A partial temp file must never linger, so remove it on either error path.
	exe := executablePath()
	tmp := tempPathBesideExe(exe)
	if err := os.WriteFile(tmp, asset, 0755); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("write temp binary: %w", err)
	}
	if err := apply(exe, tmp); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("apply update: %w", err)
	}

	// The download is always the upstream "latest" release, verified against that
	// release's own signed SHA256SUMS. latestVersion is the tag the update check
	// reported and is logged only for context - the trust comes from the
	// signature/hash, not from the version string.
	log.Printf("Self-update staged from the latest release (%s) at %s; restart to apply", latestVersion, exe)
	return true, nil
}
