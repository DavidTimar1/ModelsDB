package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Names of the two files a release publishes besides the binaries. The
// signature is made on the maintainer's machine and uploaded last.
const (
	SumsFile = "SHA256SUMS"
	SigFile  = "SHA256SUMS.sig"
)

// versionHeader starts the first line of SHA256SUMS. The version sits inside the
// signed bytes because a signature alone proves who published a file, not when:
// without it an old signed release could be replayed under a newer tag.
const versionHeader = "# version: "

var (
	versionRe = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	sumLineRe = regexp.MustCompile(`^([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*)$`)
)

// Sums is a parsed SHA256SUMS: the signed release version and each asset's
// SHA-256 in lowercase hex.
type Sums struct {
	Version string
	files   map[string]string
}

// FormatSums renders SHA256SUMS for version: the version header, then one
// "<hex sha256>  <name>" line per asset, sorted by name. files maps name to hex.
func FormatSums(version string, files map[string]string) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var b bytes.Buffer
	b.WriteString(versionHeader + version + "\n")
	for _, n := range names {
		fmt.Fprintf(&b, "%s  %s\n", files[n], n)
	}
	return b.Bytes()
}

// ParseSums parses SHA256SUMS strictly: the first line is the version header,
// every other line is a well-formed checksum line, names are unique, there is at
// least one asset, and the text ends in a newline. Anything else is refused,
// because this text decides which bytes replace the running program.
func ParseSums(raw []byte) (Sums, error) {
	text := string(raw)
	if !strings.HasSuffix(text, "\n") {
		return Sums{}, errors.New("SHA256SUMS does not end with a newline")
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if !strings.HasPrefix(lines[0], versionHeader) {
		return Sums{}, errors.New("SHA256SUMS has no version header on its first line")
	}
	version := strings.TrimPrefix(lines[0], versionHeader)
	if _, err := parseVersion(version); err != nil {
		return Sums{}, fmt.Errorf("SHA256SUMS version header: %w", err)
	}
	if len(lines) < 2 {
		return Sums{}, errors.New("SHA256SUMS lists no assets")
	}
	s := Sums{Version: version, files: make(map[string]string, len(lines))}
	for _, line := range lines[1:] {
		m := sumLineRe.FindStringSubmatch(line)
		if m == nil {
			return Sums{}, fmt.Errorf("SHA256SUMS has a malformed line %q", line)
		}
		if _, dup := s.files[m[2]]; dup {
			return Sums{}, fmt.Errorf("SHA256SUMS lists %s twice", m[2])
		}
		s.files[m[2]] = m[1]
	}
	return s, nil
}

// Names returns the asset names SHA256SUMS lists, sorted.
func (s Sums) Names() []string {
	names := make([]string, 0, len(s.files))
	for n := range s.files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Hash returns the recorded SHA-256 (lowercase hex) of the named asset.
func (s Sums) Hash(name string) (string, bool) {
	h, ok := s.files[name]
	return h, ok
}

// CheckAsset requires the asset bytes to hash to the SHA-256 recorded for name.
// A name with no line is refused.
func (s Sums) CheckAsset(name string, asset []byte) error {
	want, ok := s.files[name]
	if !ok {
		return fmt.Errorf("SHA256SUMS has no entry for %q", name)
	}
	sum := sha256.Sum256(asset)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("asset hash mismatch for %s: got %s, want %s", name, got, want)
	}
	return nil
}

// parseVersion parses a strict bare X.Y.Z release version. Leading zeros, a
// leading "v", a suffix and a missing part are all refused.
func parseVersion(v string) ([3]int, error) {
	var out [3]int
	m := versionRe.FindStringSubmatch(v)
	if m == nil {
		return out, fmt.Errorf("%q is not a release version of the form X.Y.Z", v)
	}
	for i := range out {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return out, fmt.Errorf("%q is not a release version of the form X.Y.Z", v)
		}
		out[i] = n
	}
	return out, nil
}

// CheckNewer returns nil only when signed is strictly newer than installed.
// Equal, older or malformed on either side is an error, so a replayed older
// release is refused.
func CheckNewer(installed, signed string) error {
	have, err := parseVersion(installed)
	if err != nil {
		return fmt.Errorf("running version: %w", err)
	}
	got, err := parseVersion(signed)
	if err != nil {
		return fmt.Errorf("signed version: %w", err)
	}
	for i := range have {
		switch {
		case got[i] > have[i]:
			return nil
		case got[i] < have[i]:
			return fmt.Errorf("signed release %s is older than the running version %s; refusing a downgrade", signed, installed)
		}
	}
	return fmt.Errorf("signed release %s is not newer than the running version %s", signed, installed)
}
