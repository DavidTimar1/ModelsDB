package selfupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// errSigningKeyMissing is returned when no real public key is embedded, so the
// signature cannot be checked and the update must be refused.
var errSigningKeyMissing = errors.New("release signing key not configured")

// decodeSignature parses the base64 text of a SHA256SUMS.sig file into the raw
// 64-byte ed25519 signature.
func decodeSignature(raw []byte) ([]byte, error) {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signature has wrong size: %d (want %d)", len(sig), ed25519.SignatureSize)
	}
	return sig, nil
}

// verifySums checks the ed25519 signature over the raw SHA256SUMS bytes using
// the embedded release public key. It fails closed: a missing key, an
// unparseable key, or an invalid signature all return an error.
func verifySums(sums, sig []byte) error {
	if !pubKeyConfigured() {
		return errSigningKeyMissing
	}
	return verifySumsWithKey(releasePubKeyB64, sums, sig)
}

// verifySumsWithKey verifies sig over sums using the given base64 ed25519 public
// key. It is the key-parameterised core of verifySums.
func verifySumsWithKey(pubB64 string, sums, sig []byte) error {
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

// expectedHash returns the lowercase hex SHA-256 recorded for assetName in the
// sha256sum-format SHA256SUMS content (lines of "<hex>  <name>").
func expectedHash(sums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		// sha256sum marks binary-mode entries with a leading '*' on the name.
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no SHA256SUMS entry for %q", assetName)
}

// verifyAsset checks that the asset bytes hash to the SHA-256 recorded for
// assetName in the (already signature-verified) SHA256SUMS content.
func verifyAsset(asset, sums []byte, assetName string) error {
	want, err := expectedHash(sums, assetName)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(asset)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("asset hash mismatch for %s: got %s, want %s", assetName, got, want)
	}
	return nil
}
