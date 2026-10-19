// Command sign is the maintainer's release helper for the self-update signing
// scheme. It is NOT part of the shipped binary and is not built by build.sh.
//
// Releases are signed on the maintainer's own machine, never in CI, so no
// signing key exists in GitHub. CI builds a DRAFT release; "publish" signs it
// here and makes it public.
//
//	sign keygen  -seed-out FILE
//	    Generate an ed25519 key pair once. The private seed (hex) goes to FILE,
//	    created with mode 0600, never overwritten, refused inside any git work
//	    tree, and never printed. Only the PUBLIC key is printed: paste it into
//	    internal/selfupdate/pubkey.go.
//
//	sign sums    -version X.Y.Z -out FILE ASSET...
//	    Write SHA256SUMS: a version header, then the SHA-256 of each asset. CI
//	    runs this; it needs no key.
//
//	sign publish -tag vX.Y.Z -commit HASH -key FILE [-repo OWNER/REPO]
//	    Take the draft release CI built, check its binaries against SHA256SUMS,
//	    require the tag to point at the commit you name with -commit, rebuild
//	    the binaries from that commit inside a bubblewrap sandbox that cannot
//	    see the key, and require identical hashes,
//	    sign SHA256SUMS with the key file, upload the signature and publish the
//	    draft, using the maintainer's own gh login.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"modelsdb/internal/selfupdate"
)

// hostOS is the operating system this program runs on. A variable so a test can
// exercise the Windows branch.
var hostOS = runtime.GOOS

// embeddedPubKey returns the base64 public key compiled into the app. It is a
// variable so a test can supply its own.
var embeddedPubKey = selfupdate.ReleasePublicKeyB64

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "sign: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: sign keygen|sums|publish [flags]")
	}
	switch args[0] {
	case "keygen":
		return cmdKeygen(args[1:], stdout)
	case "sums":
		return cmdSums(args[1:], stdout)
	case "publish":
		return cmdPublish(args[1:], stdout, getenv)
	default:
		return fmt.Errorf("unknown subcommand %q (want keygen, sums or publish)", args[0])
	}
}

// cmdKeygen creates a key pair. The seed goes to a new file and is not printed;
// only the public key is printed.
func cmdKeygen(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	seedOut := fs.String("seed-out", "", "REQUIRED: file for the private seed (mode 0600, must not exist, must be outside any git work tree)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *seedOut == "" {
		return errors.New("-seed-out is required: name a file outside any repository for the private seed")
	}
	if inside, err := insideGitWorkTree(*seedOut); err != nil {
		return err
	} else if inside {
		return fmt.Errorf("%s is inside a git work tree; the private seed must never be kept in a repository", *seedOut)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	f, err := os.OpenFile(*seedOut, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create %s (it must not already exist): %w", *seedOut, err)
	}
	_, werr := f.WriteString(hex.EncodeToString(priv.Seed()) + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(*seedOut)
		return fmt.Errorf("write %s: %w", *seedOut, werr)
	}
	fmt.Fprintf(stdout, "private seed written to %s (mode 0600; it is not shown here)\n", *seedOut)
	fmt.Fprintln(stdout, "PUBLIC key, paste as releasePubKeyB64 in internal/selfupdate/pubkey.go:")
	fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(pub))
	return nil
}

// cmdSums writes SHA256SUMS for the named release files.
func cmdSums(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sums", flag.ContinueOnError)
	version := fs.String("version", "", "the release version, X.Y.Z")
	out := fs.String("out", "SHA256SUMS", "file to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("name at least one release file to checksum")
	}
	files := make(map[string]string, fs.NArg())
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.Base(path)] = sha256Hex(data)
	}
	body := selfupdate.FormatSums(*version, files)
	if _, err := selfupdate.ParseSums(body); err != nil {
		return fmt.Errorf("the SHA256SUMS this would write is not valid: %w", err)
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s for release %s\n", *out, *version)
	return nil
}

// loadSigningKey reads the hex seed file and returns the private key. The file
// must be a regular file (not a link) that no other user can read. No error
// text ever contains the file's contents.
func loadSigningKey(path string, warn io.Writer) (ed25519.PrivateKey, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("key file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("key file %s is not a regular file", path)
	}
	switch {
	case hostOS == "windows":
		// Windows reports a made-up mode (0666 or 0444) for every file, so the
		// mode says nothing about who can read it; access control lists decide.
		fmt.Fprintf(warn, "warning: cannot check who can read %s on Windows; make sure only your account can\n", path)
	case fi.Mode().Perm()&0o077 != 0:
		return nil, fmt.Errorf("key file %s is readable by other users (mode %o); run chmod 600 on it", path, fi.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("key file %s does not hold a %d-byte hex seed", path, ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// checkKeyMatchesEmbedded refuses a key whose public half is not the one the
// app embeds, since no installed app could verify what it signs.
func checkKeyMatchesEmbedded(priv ed25519.PrivateKey) error {
	want := strings.TrimSpace(embeddedPubKey())
	if want == "" {
		return errors.New("no public key is embedded in internal/selfupdate/pubkey.go; nothing could verify a signature")
	}
	got := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if got != want {
		return errors.New("the signing key does not match the public key embedded in internal/selfupdate/pubkey.go; " +
			"no installed app could verify this release, so it is not signed")
	}
	return nil
}

// insideGitWorkTree reports whether path (which may not exist yet) lies inside a
// git work tree: some folder above it holds a .git entry.
func insideGitWorkTree(path string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	dir := filepath.Dir(abs)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false, nil
		}
		dir = parent
	}
}
