package selfupdate

// releasePubKeyB64 is the base64-encoded ed25519 PUBLIC key that release
// SHA256SUMS files are signed with. Self-update verifies the checksums file's
// signature against this key before it downloads or trusts any binary.
//
// The matching private key lives only on the maintainer's machine, in a 0600
// file outside every repository, and never in GitHub. CI builds a draft release
// and `go run ./cmd/sign publish` signs and publishes it there. A fork that wants
// working self-update makes its own pair with
//
//	go run ./cmd/sign keygen -seed-out <file outside any repository>
//
// and pastes the printed public key here.
//
// An empty value means "not configured": self-update then refuses everything.
// There is deliberately one key and no second slot. Replacing this value strands
// every install built with the old one, which must be downloaded by hand once.
const releasePubKeyB64 = "1jA802CwPXji2atEF0sC1Kl9MRhCOSZACTCXmGLPq0k="

// ReleasePublicKeyB64 returns the embedded base64 release public key, so the
// maintainer's signing command can check its key against the one the app trusts.
func ReleasePublicKeyB64() string {
	return releasePubKeyB64
}

// pubKeyConfigured reports whether a real signing public key has been embedded.
func pubKeyConfigured() bool {
	return releasePubKeyB64 != ""
}

// ReleaseKeyConfigured reports whether a real release signing public key is
// embedded (i.e. self-update can actually verify a download), so callers can
// avoid offering an in-app update that would only fail closed.
func ReleaseKeyConfigured() bool {
	return pubKeyConfigured()
}
