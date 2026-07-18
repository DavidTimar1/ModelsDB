// Package app - "add to PATH" menu action. Makes the modelsdb command runnable
// from any terminal by either symlinking the binary into a directory already on
// PATH (Unix) or adding the binary's own folder to PATH (a shell rc file on Unix,
// the user environment in the registry on Windows). The two Unix methods are
// mutually exclusive: installing one removes the other, and only ever the
// artifacts this app created - a fenced block in the rc file, the fixed symlink,
// or the binary's own folder in the Windows user Path. It never edits a PATH
// entry or link the user made themselves.
//
// The platform-specific I/O lives in pathinstall_unix.go / pathinstall_windows.go;
// the pure text helpers below are shared and unit-tested on every platform.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Fence markers around the PATH line this app manages in a shell rc file. Removal
// deletes only the lines between (and including) these markers, so anything the
// user wrote in the same file is left untouched.
const (
	pathBlockBegin = "# >>> modelsdb PATH (managed - do not edit this block) >>>"
	pathBlockEnd   = "# <<< modelsdb PATH (managed) <<<"
)

// rcBlockPresent reports whether content already carries the managed block.
func rcBlockPresent(content string) bool {
	return strings.Contains(content, pathBlockBegin)
}

// rcBlockRemove returns content with the managed block (markers inclusive, plus a
// single blank line immediately before it, if any) removed. Content with no block
// is returned unchanged.
func rcBlockRemove(content string) string {
	start := strings.Index(content, pathBlockBegin)
	if start == -1 {
		return content
	}
	endMarker := strings.Index(content[start:], pathBlockEnd)
	if endMarker == -1 {
		// Truncated block (no end marker): drop from the begin marker onward.
		return strings.TrimRight(content[:start], "\n") + "\n"
	}
	end := start + endMarker + len(pathBlockEnd)
	// Absorb the newline that ends the end-marker line.
	if end < len(content) && content[end] == '\n' {
		end++
	}
	before := strings.TrimRight(content[:start], "\n")
	after := content[end:]
	switch {
	case before == "" && after == "":
		return ""
	case before == "":
		return strings.TrimLeft(after, "\n")
	case after == "":
		return before + "\n"
	default:
		return before + "\n" + strings.TrimLeft(after, "\n")
	}
}

// rcBlockAdd returns content with a fresh managed block appended that prepends dir
// to PATH. Any existing managed block is removed first, so re-running updates the
// directory in place instead of stacking duplicates.
func rcBlockAdd(content, dir string) string {
	base := rcBlockRemove(content)
	block := pathBlockBegin + "\n" +
		fmt.Sprintf("export PATH=\"%s:$PATH\"", dir) + "\n" +
		pathBlockEnd + "\n"
	if base == "" {
		return block
	}
	return strings.TrimRight(base, "\n") + "\n\n" + block
}

// pathListContains reports whether a PATH-style list (separator-joined) already
// holds dir. Comparison is case-insensitive and ignores trailing slashes, which
// suits Windows user Path handling.
func pathListContains(list, sep, dir string) bool {
	want := strings.TrimRight(dir, `\/`)
	for _, part := range strings.Split(list, sep) {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(part), `\/`), want) {
			return true
		}
	}
	return false
}

// pathListAdd appends dir to a PATH-style list if absent, returning the new list
// and whether it changed.
func pathListAdd(list, sep, dir string) (string, bool) {
	if pathListContains(list, sep, dir) {
		return list, false
	}
	if strings.TrimSpace(list) == "" {
		return dir, true
	}
	return strings.TrimRight(list, sep) + sep + dir, true
}

// pathListRemove returns the list with any entry equal to dir removed, and
// whether it changed.
func pathListRemove(list, sep, dir string) (string, bool) {
	want := strings.TrimRight(dir, `\/`)
	kept := make([]string, 0)
	changed := false
	for _, part := range strings.Split(list, sep) {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(part), `\/`), want) {
			changed = true
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, sep), changed
}

// tildeify shortens a path under the home directory to a ~-relative form for
// display. It returns p unchanged if it is not under home.
func tildeify(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// readLine reads one trimmed line from standard input via the shared menu reader
// (see menuIn - a separate reader would steal buffered input).
func readLine() string {
	line, _ := menuIn.ReadString('\n')
	return strings.TrimSpace(line)
}

func onOff(on bool, detail string) string {
	if !on {
		return "not set"
	}
	if detail != "" {
		return "set -> " + detail
	}
	return "set"
}

// runPathInstall drives the "add to PATH" submenu: show current state, let the
// user pick a method (or uninstall), apply it, and remove the competing method.
func runPathInstall() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Cannot locate this executable: %v\n", err)
		return
	}
	dir := filepath.Dir(exe)

	first := true
	for {
		if first {
			fmt.Println()
			first = false
		} else {
			menuRule()
		}
		fmt.Println("Make 'modelsdb' runnable from any terminal.")
		fmt.Printf("  Binary: %s\n", exe)
		if supportsLink {
			on, target := linkStatus()
			fmt.Printf("  Symlink (%s): %s\n", tildeify(symlinkPath()), onOff(on, tildeify(target)))
		}
		on, _ := shellPathStatus(dir)
		fmt.Printf("  PATH entry (%s): %s\n", shellPathTargetLabel(), onOff(on, ""))
		fmt.Println("(Takes effect in NEW terminals - the current one keeps its old PATH.)")
		fmt.Println()

		if supportsLink {
			fmt.Printf("  1) Symlink into %s\n", tildeify(filepath.Dir(symlinkPath())))
			fmt.Printf("  2) Add this folder to PATH (%s)\n", shellPathTargetLabel())
			fmt.Println("  3) Uninstall (remove both)")
			fmt.Println("  4) Back")
			switch readLine() {
			case "1":
				applyLink(exe)
			case "2":
				applyShellPath(dir)
			case "3":
				applyUninstall(dir)
			case "4", "":
				return
			default:
				fmt.Println("Enter a number from 1 to 4.")
			}
		} else {
			fmt.Printf("  1) Add this folder to PATH (%s)\n", shellPathTargetLabel())
			fmt.Println("  2) Uninstall")
			fmt.Println("  3) Back")
			switch readLine() {
			case "1":
				applyShellPath(dir)
			case "2":
				applyUninstall(dir)
			case "3", "":
				return
			default:
				fmt.Println("Enter a number from 1 to 3.")
			}
		}
	}
}

func applyLink(exe string) {
	if err := removeShellPath(filepath.Dir(exe)); err != nil {
		fmt.Printf("Warning: could not remove the PATH entry: %v\n", err)
	}
	if err := installLink(exe); err != nil {
		fmt.Printf("Failed to create the symlink: %v\n", err)
		return
	}
	fmt.Printf("Symlinked %s -> %s\n", tildeify(symlinkPath()), exe)
	fmt.Printf("Open a NEW terminal and run: modelsdb\n")
	fmt.Printf("(If it is not found, ensure %s is on your PATH - most distros add it at login once the folder exists, so a re-login may be needed.)\n", tildeify(filepath.Dir(symlinkPath())))
}

func applyShellPath(dir string) {
	if supportsLink {
		if err := removeLink(); err != nil {
			fmt.Printf("Warning: could not remove the symlink: %v\n", err)
		}
	}
	if err := installShellPath(dir); err != nil {
		fmt.Printf("Failed to add the PATH entry: %v\n", err)
		return
	}
	fmt.Printf("Added %s to PATH via %s\n", dir, shellPathTargetLabel())
	fmt.Println("Open a NEW terminal and run: modelsdb")
}

func applyUninstall(dir string) {
	removed := false
	if supportsLink {
		if on, _ := linkStatus(); on {
			if err := removeLink(); err != nil {
				fmt.Printf("Warning: could not remove the symlink: %v\n", err)
			} else {
				removed = true
			}
		}
	}
	if on, _ := shellPathStatus(dir); on {
		if err := removeShellPath(dir); err != nil {
			fmt.Printf("Warning: could not remove the PATH entry: %v\n", err)
		} else {
			removed = true
		}
	}
	if removed {
		fmt.Println("Removed. 'modelsdb' will no longer be on PATH in new terminals.")
	} else {
		fmt.Println("Nothing to remove - no managed PATH entry or symlink was set.")
	}
}
