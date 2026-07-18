package selfupdate

import "runtime"

// assetNameFor maps an OS/arch pair to the release asset filename, and reports
// whether that combination is published. The release pipeline builds only
// linux/amd64 ("modelsdb") and windows/amd64 ("modelsdb.exe"); every other
// combination is unsupported and self-update disables cleanly for it.
func assetNameFor(goos, goarch string) (string, bool) {
	if goarch != "amd64" {
		return "", false
	}
	switch goos {
	case "linux":
		return "modelsdb", true
	case "windows":
		return "modelsdb.exe", true
	default:
		return "", false
	}
}

// assetName returns the release asset filename for the running platform.
func assetName() (string, bool) {
	return assetNameFor(runtime.GOOS, runtime.GOARCH)
}

// Supported reports whether self-update can run on this build's OS/arch.
func Supported() bool {
	_, ok := assetName()
	return ok
}
