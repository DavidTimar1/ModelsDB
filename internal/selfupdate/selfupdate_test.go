package selfupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestAssetNameFor(t *testing.T) {
	cases := []struct {
		goos, goarch string
		wantName     string
		wantOK       bool
	}{
		{"linux", "amd64", "modelsdb", true},
		{"windows", "amd64", "modelsdb.exe", true},
		{"darwin", "amd64", "", false},
		{"linux", "arm64", "", false},
		{"windows", "386", "", false},
		{"plan9", "amd64", "", false},
	}
	for _, c := range cases {
		name, ok := assetNameFor(c.goos, c.goarch)
		if name != c.wantName || ok != c.wantOK {
			t.Errorf("assetNameFor(%q,%q) = (%q,%v), want (%q,%v)",
				c.goos, c.goarch, name, ok, c.wantName, c.wantOK)
		}
	}
}

// sumsFor builds a sha256sum-format SHA256SUMS body for the given asset bytes.
func sumsFor(assetName string, asset []byte) []byte {
	sum := sha256.Sum256(asset)
	return []byte(hex.EncodeToString(sum[:]) + "  " + assetName + "\n")
}

func TestVerifyGoodSignatureAndHashPass(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	asset := []byte("pretend binary contents")
	sums := sumsFor("modelsdb", asset)
	sig := ed25519.Sign(priv, sums)

	if err := verifySumsWithKey(pubB64, sums, sig); err != nil {
		t.Fatalf("good signature should verify: %v", err)
	}
	if err := verifyAsset(asset, sums, "modelsdb"); err != nil {
		t.Fatalf("matching hash should verify: %v", err)
	}
}

func TestVerifyTamperedSumsFails(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	asset := []byte("pretend binary contents")
	sums := sumsFor("modelsdb", asset)
	sig := ed25519.Sign(priv, sums)

	// Flip a byte in the checksums AFTER signing: the signature no longer matches.
	tampered := make([]byte, len(sums))
	copy(tampered, sums)
	tampered[0] ^= 0xFF
	if err := verifySumsWithKey(pubB64, tampered, sig); err == nil {
		t.Fatal("tampered SHA256SUMS must fail signature verification")
	}
}

func TestVerifyBadSignatureFails(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	asset := []byte("pretend binary contents")
	sums := sumsFor("modelsdb", asset)

	// A signature made with a DIFFERENT private key must not verify against pub.
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	badSig := ed25519.Sign(otherPriv, sums)
	if err := verifySumsWithKey(pubB64, sums, badSig); err == nil {
		t.Fatal("signature from a different key must fail verification")
	}
}

func TestVerifyWrongAssetHashFails(t *testing.T) {
	asset := []byte("pretend binary contents")
	sums := sumsFor("modelsdb", asset)

	// The bytes on disk differ from what SHA256SUMS records for the asset.
	corrupted := []byte("different bytes than were signed")
	if err := verifyAsset(corrupted, sums, "modelsdb"); err == nil {
		t.Fatal("mismatched asset bytes must fail hash verification")
	}

	// A checksums file with no line for the asset must also fail.
	if err := verifyAsset(asset, sumsFor("other", asset), "modelsdb"); err == nil {
		t.Fatal("missing SHA256SUMS entry must fail")
	}
}

func TestVerifySumsFailsClosedWithoutEmbeddedKey(t *testing.T) {
	// The shipped placeholder key is empty, so the top-level verifySums (which
	// reads the embedded key) must refuse rather than accept anything.
	if pubKeyConfigured() {
		t.Skip("a real signing key is embedded; placeholder-key path not exercised")
	}
	if err := verifySums([]byte("x"), make([]byte, ed25519.SignatureSize)); err != errSigningKeyMissing {
		t.Fatalf("verifySums without a key = %v, want errSigningKeyMissing", err)
	}
}

func TestDecodeSignatureRoundTrip(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, []byte("data"))
	raw := []byte("  " + base64.StdEncoding.EncodeToString(sig) + "\n")
	got, err := decodeSignature(raw)
	if err != nil {
		t.Fatalf("decodeSignature: %v", err)
	}
	if string(got) != string(sig) {
		t.Fatal("decoded signature does not match the original")
	}

	if _, err := decodeSignature([]byte("not-base64!!")); err == nil {
		t.Fatal("invalid base64 must error")
	}
	if _, err := decodeSignature([]byte(base64.StdEncoding.EncodeToString([]byte("too short")))); err == nil {
		t.Fatal("wrong-size signature must error")
	}
}
