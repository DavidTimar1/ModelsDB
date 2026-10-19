package selfupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

// hexOf is the lowercase hex SHA-256 of data.
func hexOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// newKey returns a fresh key pair and the public half as base64.
func newKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, base64.StdEncoding.EncodeToString(pub)
}

// sumsText is a SHA256SUMS body for one asset at the given version.
func sumsText(version, assetName string, asset []byte) []byte {
	return FormatSums(version, map[string]string{assetName: hexOf(asset)})
}

func TestEmbeddedKeyIsAValidEd25519PublicKey(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(ReleasePublicKeyB64())
	if err != nil {
		t.Fatalf("embedded key is not base64: %v", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		t.Fatalf("embedded key is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
}

func TestVerifySumsWithKey(t *testing.T) {
	priv, pubB64 := newKey(t)
	sums := sumsText("1.2.3", "modelsdb", []byte("binary"))
	sig := ed25519.Sign(priv, sums)

	if err := VerifySumsWithKey(pubB64, sums, sig); err != nil {
		t.Fatalf("good signature should verify: %v", err)
	}

	tampered := append([]byte(nil), sums...)
	tampered[0] ^= 0xFF
	if err := VerifySumsWithKey(pubB64, tampered, sig); err == nil {
		t.Error("tampered SHA256SUMS must fail verification")
	}

	otherPriv, _ := newKey(t)
	if err := VerifySumsWithKey(pubB64, sums, ed25519.Sign(otherPriv, sums)); err == nil {
		t.Error("signature from a different key must fail verification")
	}

	if err := VerifySumsWithKey("", sums, sig); err != errSigningKeyMissing {
		t.Errorf("empty key = %v, want errSigningKeyMissing", err)
	}
}

// The version header is inside the signed bytes, so editing it breaks the signature.
func TestHeaderTamperingFailsTheSignature(t *testing.T) {
	priv, pubB64 := newKey(t)
	signed := sumsText("1.0.0", "modelsdb", []byte("old build"))
	sig := ed25519.Sign(priv, signed)
	forged := []byte(strings.Replace(string(signed), "# version: 1.0.0", "# version: 9.0.0", 1))
	if err := VerifySumsWithKey(pubB64, forged, sig); err == nil {
		t.Fatal("a rewritten version header must fail the signature")
	}
}

func TestDecodeSignatureRoundTrip(t *testing.T) {
	priv, _ := newKey(t)
	sig := ed25519.Sign(priv, []byte("data"))
	got, err := DecodeSignature([]byte("  " + string(EncodeSignature(sig))))
	if err != nil {
		t.Fatalf("DecodeSignature: %v", err)
	}
	if string(got) != string(sig) {
		t.Fatal("decoded signature does not match the original")
	}
	if _, err := DecodeSignature([]byte("not-base64!!")); err == nil {
		t.Fatal("invalid base64 must error")
	}
	if _, err := DecodeSignature([]byte(base64.StdEncoding.EncodeToString([]byte("too short")))); err == nil {
		t.Fatal("wrong-size signature must error")
	}
}

func TestParseSumsIsStrict(t *testing.T) {
	h := strings.Repeat("a", 64)
	h2 := strings.Repeat("b", 64)
	good := "# version: 1.2.3\n" + h + "  modelsdb\n" + h2 + "  modelsdb.exe\n"
	s, err := ParseSums([]byte(good))
	if err != nil {
		t.Fatalf("good SHA256SUMS refused: %v", err)
	}
	if s.Version != "1.2.3" {
		t.Errorf("version = %q", s.Version)
	}
	if got := strings.Join(s.Names(), ","); got != "modelsdb,modelsdb.exe" {
		t.Errorf("names = %q", got)
	}
	if got, ok := s.Hash("modelsdb.exe"); !ok || got != h2 {
		t.Errorf("hash = %q, %v", got, ok)
	}

	bad := map[string]string{
		"no header":            h + "  modelsdb\n",
		"header not first":     h + "  modelsdb\n# version: 1.2.3\n",
		"malformed version":    "# version: 1.2\n" + h + "  modelsdb\n",
		"v prefix":             "# version: v1.2.3\n" + h + "  modelsdb\n",
		"suffix":               "# version: 1.2.3-rc1\n" + h + "  modelsdb\n",
		"leading zero":         "# version: 01.2.3\n" + h + "  modelsdb\n",
		"empty version":        "# version: \n" + h + "  modelsdb\n",
		"duplicate name":       "# version: 1.2.3\n" + h + "  modelsdb\n" + h2 + "  modelsdb\n",
		"no trailing newline":  "# version: 1.2.3\n" + h + "  modelsdb",
		"uppercase hash":       "# version: 1.2.3\n" + strings.ToUpper(h) + "  modelsdb\n",
		"short hash":           "# version: 1.2.3\n" + h[:60] + "  modelsdb\n",
		"one space separator":  "# version: 1.2.3\n" + h + " modelsdb\n",
		"binary-mode asterisk": "# version: 1.2.3\n" + h + " *modelsdb\n",
		"path in name":         "# version: 1.2.3\n" + h + "  ../modelsdb\n",
		"blank line":           "# version: 1.2.3\n\n" + h + "  modelsdb\n",
		"no assets":            "# version: 1.2.3\n",
		"empty":                "",
		"crlf":                 "# version: 1.2.3\r\n" + h + "  modelsdb\r\n",
	}
	for name, body := range bad {
		if _, err := ParseSums([]byte(body)); err == nil {
			t.Errorf("%s: malformed SHA256SUMS was accepted", name)
		}
	}
}

func TestCheckNewer(t *testing.T) {
	cases := []struct {
		installed, signed string
		ok                bool
	}{
		{"1.2.3", "1.2.4", true},
		{"1.2.3", "1.3.0", true},
		{"1.2.3", "2.0.0", true},
		{"1.9.0", "1.10.0", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.2", false},
		{"2.0.0", "1.99.99", false},
		{"1.2.3", "1.2", false},
		{"1.2.3", "v1.2.4", false},
		{"1.2.3", "", false},
		{"dev", "1.2.4", false},
		{"1.2.3-dev", "1.2.4", false},
		{"", "1.2.4", false},
	}
	for _, c := range cases {
		err := CheckNewer(c.installed, c.signed)
		if (err == nil) != c.ok {
			t.Errorf("CheckNewer(%q, %q) = %v, want ok=%v", c.installed, c.signed, err, c.ok)
		}
	}
}

func TestCheckAsset(t *testing.T) {
	asset := []byte("pretend binary contents")
	s, err := ParseSums(sumsText("1.0.0", "modelsdb", asset))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckAsset("modelsdb", asset); err != nil {
		t.Fatalf("matching hash should verify: %v", err)
	}
	if err := s.CheckAsset("modelsdb", []byte("different bytes")); err == nil {
		t.Fatal("mismatched asset bytes must fail")
	}
	if err := s.CheckAsset("other", asset); err == nil {
		t.Fatal("a name with no line must fail")
	}
}

// release is a fake download server: it serves files by path and records the
// order of requests.
type release struct {
	t     *testing.T
	files map[string][]byte
	mu    sync.Mutex
	hits  []string
	srv   *httptest.Server
}

func newRelease(t *testing.T, files map[string][]byte) *release {
	t.Helper()
	r := &release{t: t, files: files}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/")
		r.mu.Lock()
		r.hits = append(r.hits, name)
		r.mu.Unlock()
		data, ok := r.files[name]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *release) requested(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range r.hits {
		if h == name {
			return true
		}
	}
	return false
}

// stagerFor builds a stager that reads from r, with a throwaway "running
// executable" holding oldBytes.
func stagerFor(t *testing.T, r *release, pubB64, installed string) (stager, string) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "modelsdb")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return stager{
		base:      r.srv.URL,
		pubKeyB64: pubB64,
		installed: installed,
		asset:     "modelsdb",
		exe:       exe,
		client:    &http.Client{Timeout: 5 * time.Second},
	}, exe
}

// signedRelease serves a release signed by priv: SHA256SUMS at signedVersion,
// its signature, and the binary.
func signedRelease(t *testing.T, priv ed25519.PrivateKey, signedVersion string, binary []byte) *release {
	t.Helper()
	sums := sumsText(signedVersion, "modelsdb", binary)
	return newRelease(t, map[string][]byte{
		SumsFile:   sums,
		SigFile:    EncodeSignature(ed25519.Sign(priv, sums)),
		"modelsdb": binary,
	})
}

func assertUntouched(t *testing.T, exe string) {
	t.Helper()
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "old binary" {
		t.Fatalf("the running binary was touched: %q, %v", got, err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".modelsdb-update-*"))
	if len(left) != 0 {
		t.Fatalf("temp files were left beside the executable: %v", left)
	}
}

func TestStageInstallsANewerSignedRelease(t *testing.T) {
	priv, pubB64 := newKey(t)
	r := signedRelease(t, priv, "1.3.0", []byte("new binary"))
	s, exe := stagerFor(t, r, pubB64, "1.2.0")

	var phases []string
	staged, err := s.run("1.3.0", func(p string) { phases = append(phases, p) })
	if err != nil || !staged {
		t.Fatalf("run = (%v, %v), want staged", staged, err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new binary" {
		t.Fatalf("executable holds %q", got)
	}
	if len(phases) == 0 || phases[0] != PhaseDownloading {
		t.Errorf("phases = %v", phases)
	}
	// The announced tag may carry a leading v.
	s2, _ := stagerFor(t, r, pubB64, "1.2.0")
	if staged, err := s2.run("v1.3.0", nil); err != nil || !staged {
		t.Fatalf("a v-prefixed announced version should match: %v", err)
	}
}

func TestStageVerifiesBeforeDownloadingTheBinary(t *testing.T) {
	priv, _ := newKey(t)
	_, otherPub := newKey(t) // the app trusts a different key
	r := signedRelease(t, priv, "1.3.0", []byte("new binary"))
	s, exe := stagerFor(t, r, otherPub, "1.2.0")

	if staged, err := s.run("1.3.0", nil); err == nil || staged {
		t.Fatal("a release signed by another key must be refused")
	}
	if r.requested("modelsdb") {
		t.Fatal("the binary was downloaded before the signature was verified")
	}
	assertUntouched(t, exe)
}

// A validly signed OLDER release published under a newer tag is a replay.
func TestStageRefusesAReplayedOlderRelease(t *testing.T) {
	priv, pubB64 := newKey(t)
	r := signedRelease(t, priv, "1.0.0", []byte("old but validly signed"))
	s, exe := stagerFor(t, r, pubB64, "1.2.0")

	// The update check was fooled by a newer tag.
	staged, err := s.run("1.5.0", nil)
	if err == nil || staged {
		t.Fatal("a replayed older release must be refused")
	}
	if !strings.Contains(err.Error(), "older") {
		t.Errorf("error should say the release is older: %v", err)
	}
	if r.requested("modelsdb") {
		t.Fatal("the binary was downloaded for a refused version")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesAnEqualVersion(t *testing.T) {
	priv, pubB64 := newKey(t)
	r := signedRelease(t, priv, "1.2.0", []byte("same"))
	s, exe := stagerFor(t, r, pubB64, "1.2.0")
	if staged, err := s.run("1.2.0", nil); err == nil || staged {
		t.Fatal("an equal version must be refused")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesASignedVersionThatDiffersFromTheAnnouncedOne(t *testing.T) {
	priv, pubB64 := newKey(t)
	r := signedRelease(t, priv, "1.4.0", []byte("new binary"))
	s, exe := stagerFor(t, r, pubB64, "1.2.0")
	staged, err := s.run("1.3.0", nil)
	if err == nil || staged {
		t.Fatal("a signed version that differs from the announced one must be refused")
	}
	if !strings.Contains(err.Error(), "differs") {
		t.Errorf("error should name the mismatch: %v", err)
	}
	if r.requested("modelsdb") {
		t.Fatal("the binary was downloaded for a refused version")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesAMalformedRunningVersion(t *testing.T) {
	priv, pubB64 := newKey(t)
	r := signedRelease(t, priv, "1.3.0", []byte("new binary"))
	s, exe := stagerFor(t, r, pubB64, "dev")
	if staged, err := s.run("1.3.0", nil); err == nil || staged {
		t.Fatal("a malformed running version must be refused")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesAHashMismatch(t *testing.T) {
	priv, pubB64 := newKey(t)
	sums := sumsText("1.3.0", "modelsdb", []byte("the signed binary"))
	r := newRelease(t, map[string][]byte{
		SumsFile:   sums,
		SigFile:    EncodeSignature(ed25519.Sign(priv, sums)),
		"modelsdb": []byte("a different binary"),
	})
	s, exe := stagerFor(t, r, pubB64, "1.2.0")
	if staged, err := s.run("1.3.0", nil); err == nil || staged {
		t.Fatal("a binary that does not match its signed hash must be refused")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesWhenTheSumsHaveNoEntryForThisPlatform(t *testing.T) {
	priv, pubB64 := newKey(t)
	sums := sumsText("1.3.0", "other-asset", []byte("x"))
	r := newRelease(t, map[string][]byte{
		SumsFile: sums,
		SigFile:  EncodeSignature(ed25519.Sign(priv, sums)),
	})
	s, exe := stagerFor(t, r, pubB64, "1.2.0")
	if staged, err := s.run("1.3.0", nil); err == nil || staged {
		t.Fatal("missing entry must be refused")
	}
	if r.requested("modelsdb") {
		t.Fatal("the binary was downloaded without a signed entry")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesWithoutAnEmbeddedKey(t *testing.T) {
	priv, _ := newKey(t)
	r := signedRelease(t, priv, "1.3.0", []byte("new binary"))
	s, exe := stagerFor(t, r, "", "1.2.0")
	if _, err := s.run("1.3.0", nil); err != errSigningKeyMissing {
		t.Fatalf("run without a key = %v, want errSigningKeyMissing", err)
	}
	if len(r.hits) != 0 {
		t.Fatal("something was downloaded without a key to verify it")
	}
	assertUntouched(t, exe)
}

func TestStageRefusesAMissingSignature(t *testing.T) {
	_, pubB64 := newKey(t)
	binary := []byte("new binary")
	sums := sumsText("1.3.0", "modelsdb", binary)
	r := newRelease(t, map[string][]byte{SumsFile: sums, "modelsdb": binary})
	s, exe := stagerFor(t, r, pubB64, "1.2.0")
	if staged, err := s.run("1.3.0", nil); err == nil || staged {
		t.Fatal("an unsigned release must be refused")
	}
	if r.requested("modelsdb") {
		t.Fatal("the binary was downloaded for an unsigned release")
	}
	assertUntouched(t, exe)
}

func TestDownloadsAreSizeCapped(t *testing.T) {
	r := newRelease(t, map[string][]byte{"big": make([]byte, 2048)})
	c := &http.Client{Timeout: 5 * time.Second}
	if _, err := httpGetBytes(c, r.srv.URL+"/big", 4096); err != nil {
		t.Fatalf("a body under the cap should be read: %v", err)
	}
	if _, err := httpGetBytes(c, r.srv.URL+"/big", 2047); err == nil {
		t.Fatal("a body over the cap must be refused")
	}
	if _, err := httpGetBytes(c, r.srv.URL+"/missing", 4096); err == nil {
		t.Fatal("a 404 must be an error")
	}
}
