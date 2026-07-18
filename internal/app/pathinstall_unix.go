//go:build unix

package app

import (
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

// installLink creates (or replaces) the managed symlink pointing at exe.
func installLink(exe string) error {
	if err := os.MkdirAll(localBinDir(), 0755); err != nil {
		return err
	}
	_ = os.Remove(symlinkPath()) // replace any existing managed link
	return os.Symlink(exe, symlinkPath())
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
	return os.WriteFile(rc, []byte(rcBlockAdd(string(b), dir)), 0644)
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
	return os.WriteFile(rc, []byte(out), 0644)
}
