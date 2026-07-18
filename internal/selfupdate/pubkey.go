package selfupdate

// releasePubKeyB64 is the base64-encoded ed25519 PUBLIC key that release
// SHA256SUMS files are signed with. Self-update verifies the checksums file's
// signature against this key before trusting any downloaded binary.
//
// PLACEHOLDER: this is intentionally empty until the maintainer generates the
// real signing key pair once, with:
//
//	go run ./cmd/sign keygen
//
// Paste the printed PUBLIC key here (as the value of this constant), and add the
// printed PRIVATE key as the CI secret MODELSDB_SIGN_KEY (never commit it). The
// release workflow then signs SHA256SUMS with that private key.
//
// While this is empty, self-update fails closed by design: Supported() may still
// report true, but Stage() refuses with "release signing key not configured".
const releasePubKeyB64 = "5H1g11frf+RWrXrtuMZMrRyVszqcmPIcpVcxMhrhFw0="

// pubKeyConfigured reports whether a real signing public key has been embedded.
func pubKeyConfigured() bool {
	return releasePubKeyB64 != ""
}

// ReleaseKeyConfigured reports whether a real release signing public key is
// embedded (i.e. self-update can actually verify a download). It is false while
// pubkey.go still holds the placeholder, so callers can avoid offering an
// in-app update that would only fail closed.
func ReleaseKeyConfigured() bool {
	return pubKeyConfigured()
}
