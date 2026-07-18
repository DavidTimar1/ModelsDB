//go:build windows

package app

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// supportsLink is false on Windows: reliable symlinks need elevation or Developer
// Mode, so the only install method offered is adding the binary's folder to the
// per-user PATH in the registry.
const supportsLink = false

// The link functions exist so the shared submenu compiles on every platform; on
// Windows they are inert (the symlink option is never shown).
func localBinDir() string        { return "" }
func symlinkPath() string        { return "" }
func linkStatus() (bool, string) { return false, "" }
func installLink(exe string) error {
	return fmt.Errorf("symlink install is not supported on Windows")
}
func removeLink() error { return nil }

// shellPathTargetLabel names where the PATH method writes, for display.
func shellPathTargetLabel() string { return "your user PATH (environment)" }

// openUserEnv opens HKCU\Environment, the per-user environment block, with the
// given access rights.
func openUserEnv(access uint32) (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, `Environment`, access)
}

// readUserPath returns the current per-user PATH value ("" if unset).
func readUserPath() (string, error) {
	k, err := openUserEnv(registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return "", nil
	}
	return v, err
}

// writeUserPath stores value as the per-user PATH, keeping it expandable so
// entries like %USERPROFILE% still resolve.
func writeUserPath(value string) error {
	k, err := openUserEnv(registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetExpandStringValue("Path", value)
}

// shellPathStatus reports whether dir is already in the per-user PATH.
func shellPathStatus(dir string) (bool, string) {
	v, err := readUserPath()
	if err != nil {
		return false, "user PATH"
	}
	return pathListContains(v, ";", dir), "user PATH"
}

// installShellPath adds dir to the per-user PATH if absent.
func installShellPath(dir string) error {
	v, err := readUserPath()
	if err != nil {
		return err
	}
	next, changed := pathListAdd(v, ";", dir)
	if !changed {
		return nil
	}
	return writeUserPath(next)
}

// removeShellPath removes dir from the per-user PATH if present.
func removeShellPath(dir string) error {
	v, err := readUserPath()
	if err != nil {
		return err
	}
	next, changed := pathListRemove(v, ";", dir)
	if !changed {
		return nil
	}
	return writeUserPath(next)
}
