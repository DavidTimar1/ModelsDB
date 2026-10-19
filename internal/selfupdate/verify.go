package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// errSigningKeyMissing is returned when no real public key is embedded, so the
// signature cannot be checked and the update must be refused.
var errSigningKeyMissing = errors.New("release signing key not configured")

// DecodeSignature parses the base64 text of a SHA256SUMS.sig file into the raw
// 64-byte ed25519 signature.
func DecodeSignature(raw []byte) ([]byte, error) {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signature has wrong size: %d (want %d)", len(sig), ed25519.SignatureSize)
	}
	return sig, nil
}

// EncodeSignature renders a raw signature as the text of a SHA256SUMS.sig file.
func EncodeSignature(sig []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(sig) + "\n")
}

// VerifySumsWithKey verifies sig over the exact SHA256SUMS bytes with the given
// base64 ed25519 public key. It fails closed: an empty or unparseable key and an
// invalid signature all return an error.
func VerifySumsWithKey(pubB64 string, sums, sig []byte) error {
	if strings.TrimSpace(pubB64) == "" {
		return errSigningKeyMissing
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("public key has wrong size: %d (want %d)", len(pub), ed25519.PublicKeySize)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), sums, sig) {
		return errors.New("SHA256SUMS signature verification failed")
	}
	return nil
}
