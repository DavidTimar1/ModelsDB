package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"modelsdb/internal/selfupdate"
)

// verifyRebuild rebuilds the release binaries from the tag's commit and requires
// each hash to equal the one SHA256SUMS lists. It also requires the key that will
// sign to be the public key that commit embeds, since the signed binary will
// trust that key for every later update, not the key in the operator's own tree.
// keyPath is the signing key file, which the build is kept from reading.
// scratch is an existing folder; everything made here lives inside it.
func verifyRebuild(stdout io.Writer, repo, commit, keyPath string, sums selfupdate.Sums, priv ed25519.PrivateKey, scratch string) error {
	src, err := os.MkdirTemp(scratch, "src-")
	if err != nil {
		return err
	}
	if err := fetchSource(repo, commit, src); err != nil {
		return err
	}
	embedded, err := embeddedKeyAt(src)
	if err != nil {
		return err
	}
	if got := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)); got != strings.TrimSpace(embedded) {
		return errors.New("the signing key does not match the public key embedded in the tag's own commit; " +
			"a binary built from it would not trust this key, so it is not signed")
	}
	version, err := versionAt(src)
	if err != nil {
		return fmt.Errorf("read the tag's VERSION: %w", err)
	}
	if version != sums.Version {
		return fmt.Errorf("the tag's commit declares version %s but %s names %s; refusing to sign", version, selfupdate.SumsFile, sums.Version)
	}
	out, err := os.MkdirTemp(scratch, "rebuilt-")
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "rebuilding %s from commit %s with %s\n", strings.Join(sums.Names(), " and "), commit, runtime.Version())
	if err := buildBinaries(buildJob{Src: src, Version: sums.Version, OutDir: out, KeyPath: keyPath, Names: sums.Names()}); err != nil {
		return err
	}
	for _, name := range sums.Names() {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			return fmt.Errorf("rebuilt %s: %w", name, err)
		}
		want, _ := sums.Hash(name)
		if got := sha256Hex(data); got != want {
			return fmt.Errorf("%s rebuilt from commit %s hashes to %s but the draft has %s; "+
				"the draft is not what this source builds with %s (a different Go toolchain in CI gives different bytes); refusing to sign",
				name, commit, got, want, runtime.Version())
		}
	}
	fmt.Fprintln(stdout, "rebuilt binaries match the draft byte for byte")
	return nil
}

// recheckDraft downloads SHA256SUMS and every binary of the draft again and
// requires them to equal what was verified, and the release to still be a draft.
func recheckDraft(host ghHost, repo, tag, scratch string, sumsRaw []byte, hashes map[string]string) error {
	rel, err := host.view(repo, tag)
	if err != nil {
		return err
	}
	if !rel.IsDraft {
		return fmt.Errorf("release %s became public while it was being signed", tag)
	}
	dir, err := os.MkdirTemp(scratch, "recheck-")
	if err != nil {
		return err
	}
	again, err := host.fetchDraftFile(repo, tag, dir, selfupdate.SumsFile)
	if err != nil {
		return err
	}
	if !bytes.Equal(again, sumsRaw) {
		return fmt.Errorf("%s changed in the draft after it was verified", selfupdate.SumsFile)
	}
	for name, want := range hashes {
		data, err := host.fetchDraftFile(repo, tag, dir, name)
		if err != nil {
			return err
		}
		if sha256Hex(data) != want {
			return fmt.Errorf("%s changed in the draft after it was verified", name)
		}
	}
	return nil
}
