//go:build unix

package app

import (
	"fmt"
	"modelsdb/internal/paths"
	"os"
	"path/filepath"
	"strings"
)

// supportsLink is true on Unix: a symlink into ~/.local/bin is one of the two
// install methods offered.
const supportsLink = true

// localBinDir is the per-user executables directory, on PATH by convention on
// modern distros.
func localBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// symlinkPath is the fixed location of the managed symlink.
func symlinkPath() string { return filepath.Join(localBinDir(), "modelsdb") }

// linkStatus reports whether the managed symlink exists and where it points.
func linkStatus() (bool, string) {
	p := symlinkPath()
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return false, ""
	}
	dest, err := os.Readlink(p)
	if err != nil {
		return false, ""
	}
	return true, dest
}

// installLink points the managed symlink at exe. It only ever replaces a
// symlink that names a modelsdb binary, and it refuses to touch a regular file at
// the link path: the program is commonly installed at that very path, and
// replacing it would delete the binary being run. The new link is made beside the
// old one and renamed over it, so the path never stands empty.
func installLink(exe string) error {
	if err := os.MkdirAll(localBinDir(), 0755); err != nil {
		return err
	}
	link := symlinkPath()
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s already exists and is not a symlink, so it is left alone (if it is this program, it is already on PATH)", link)
		}
		dest, err := os.Readlink(link)
		if err != nil {
			return err
		}
		if !strings.Contains(filepath.Base(dest), "modelsdb") {
			return fmt.Errorf("%s is a symlink to %s, which is not a modelsdb binary, so it is left alone", link, dest)
		}
		if sameFile(link, exe) {
			return nil
		}
	}
	tmp := link + ".new"
	os.Remove(tmp)
	if err := os.Symlink(exe, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// sameFile reports whether two paths resolve to the same existing file.
func sameFile(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// removeLink deletes the managed symlink, but only when it is in fact a symlink
// to a modelsdb binary - never a real file the user placed there.
func removeLink() error {
	on, dest := linkStatus()
	if !on {
		return nil
	}
	if !strings.Contains(filepath.Base(dest), "modelsdb") {
		return nil
	}
	return os.Remove(symlinkPath())
}

// shellRcFile returns the shell startup file to edit, chosen from $SHELL: bash ->
// ~/.bashrc, zsh -> ~/.zshrc, anything else -> ~/.profile (login, shell-agnostic).
func shellRcFile() string {
	home, _ := os.UserHomeDir()
	switch sh := os.Getenv("SHELL"); {
	case strings.Contains(sh, "zsh"):
		return filepath.Join(home, ".zshrc")
	case strings.Contains(sh, "bash"):
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".profile")
	}
}

// shellPathTargetLabel names the file the PATH method edits, for display.
func shellPathTargetLabel() string { return tildeify(shellRcFile()) }

// shellPathStatus reports whether the managed PATH block is present. dir is
// unused on Unix (the block is identified by its markers, not its contents).
func shellPathStatus(dir string) (bool, string) {
	rc := shellRcFile()
	b, err := os.ReadFile(rc)
	if err != nil {
		return false, rc
	}
	return rcBlockPresent(string(b)), rc
}

// installShellPath writes (or refreshes) the managed PATH block that prepends dir.
func installShellPath(dir string) error {
	rc := shellRcFile()
	b, _ := os.ReadFile(rc) // a missing file reads as empty, which is fine
	return paths.WriteFileAtomic(rc, []byte(rcBlockAdd(string(b), dir)), 0644)
}

// removeShellPath strips the managed PATH block. dir is unused on Unix.
func removeShellPath(dir string) error {
	rc := shellRcFile()
	b, err := os.ReadFile(rc)
	if err != nil {
		return nil
	}
	out := rcBlockRemove(string(b))
	if out == string(b) {
		return nil
	}
	return paths.WriteFileAtomic(rc, []byte(out), 0644)
}
