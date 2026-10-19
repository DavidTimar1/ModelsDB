package paths

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces the file at path with data so that a crash or a full
// disk leaves either the old content or the new, never a truncated file. It
// lives in this lowest-level package so the config writer here and every
// higher layer share one definition.
//
// A path that is a symbolic link is written through to its target, so a dotfile
// kept as a link into another folder stays a link. An existing file keeps its
// permission bits; a new file gets perm.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
