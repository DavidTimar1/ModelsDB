// Command sign is a maintainer/CI helper for the self-update signing scheme. It
// is NOT part of the shipped binary and is not built by build.sh.
//
// Two subcommands:
//
//	sign keygen        Generate a fresh ed25519 key pair and print both keys
//	                   (base64) to stdout: the PUBLIC key to embed in
//	                   internal/selfupdate/pubkey.go, and the PRIVATE key to add
//	                   as the CI secret MODELSDB_SIGN_KEY. Run this once.
//
//	sign sums <file>   Read the base64 ed25519 private key from the env var
//	                   MODELSDB_SIGN_KEY and write "<file>.sig" (base64 detached
//	                   signature over the file's bytes). Used by the release
//	                   workflow to sign dist/SHA256SUMS.
//
// The private key is never printed except by keygen's explicit stdout output.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "keygen":
		keygen()
	case "sums":
		if len(os.Args) < 3 {
			fatal("usage: sign sums <file>")
		}
		signSums(os.Args[2])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  sign keygen        generate an ed25519 key pair (public for source, private for CI secret)")
	fmt.Fprintln(os.Stderr, "  sign sums <file>   write <file>.sig using MODELSDB_SIGN_KEY (base64 private key)")
	os.Exit(2)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

func keygen() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal("generate key: " + err.Error())
	}
	fmt.Println("# ed25519 release signing key pair")
	fmt.Println("#")
	fmt.Println("# PUBLIC key -> paste as releasePubKeyB64 in internal/selfupdate/pubkey.go:")
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	fmt.Println("#")
	fmt.Println("# PRIVATE key -> add as CI secret MODELSDB_SIGN_KEY (never commit):")
	fmt.Println(base64.StdEncoding.EncodeToString(priv))
}

func signSums(path string) {
	keyB64 := strings.TrimSpace(os.Getenv("MODELSDB_SIGN_KEY"))
	if keyB64 == "" {
		fatal("MODELSDB_SIGN_KEY is not set (base64 ed25519 private key required)")
	}
	priv, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		fatal("decode MODELSDB_SIGN_KEY: " + err.Error())
	}
	if len(priv) != ed25519.PrivateKeySize {
		fatal(fmt.Sprintf("private key has wrong size: %d (want %d)", len(priv), ed25519.PrivateKeySize))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("read " + path + ": " + err.Error())
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), data)
	out := base64.StdEncoding.EncodeToString(sig) + "\n"
	if err := os.WriteFile(path+".sig", []byte(out), 0644); err != nil {
		fatal("write " + path + ".sig: " + err.Error())
	}
	fmt.Printf("wrote %s.sig (%d-byte ed25519 signature, base64)\n", path, len(sig))
}
