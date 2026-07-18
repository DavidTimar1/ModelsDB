package selfupdate

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
)

// downloadTimeout bounds a single asset download. A release binary is tens of
// megabytes, so this is generous compared with the update-check timeout.
const downloadTimeout = 5 * time.Minute

// httpGetBytes fetches url and returns the whole body. Requests send only Go's
// default User-Agent - no app- or owner-identifying header - matching the
// update checker's privacy stance.
func httpGetBytes(url string) ([]byte, error) {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// tempPathBesideExe returns a temp file path in the same directory as the
// running executable. Placing it there keeps it on the same filesystem as the
// exe, so the swap rename is atomic (rename cannot cross filesystems).
func tempPathBesideExe(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), ".modelsdb-update.tmp")
}
