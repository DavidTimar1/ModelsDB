//go:build unix

package selfupdate

import "os"

// apply swaps the verified temp binary into the running executable's path. On
// unix a plain rename over the still-running executable is safe: the kernel
// keeps the already-loaded program image mapped, and the next launch reads the
// new file. Same-directory rename makes it atomic.
func apply(self, temp string) error {
	return os.Rename(temp, self)
}
