package selfupdate

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// downloadTimeout bounds a single asset download. A release binary is tens of
// megabytes, so this is generous compared with the update-check timeout.
const downloadTimeout = 5 * time.Minute

// Size caps for the three downloads. They bound memory use against a hostile or
// broken server; a real release binary is tens of megabytes.
const (
	maxSumsBytes   = 64 << 10
	maxSigBytes    = 16 << 10
	maxBinaryBytes = 256 << 20
)

// httpGetBytes fetches url and returns the whole body, refusing one longer than
// limit bytes. The client is expected to come from shared.NewHTTPClient, which
// sends the one plain browser User-Agent and no app- or owner-identifying header.
func httpGetBytes(client *http.Client, url string, limit int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("GET %s: body is larger than the %d byte limit", url, limit)
	}
	return body, nil
}

// writeTempBeside writes data to a NEW uniquely named file in the same
// directory as the running executable and returns its path. The same directory
// keeps it on the exe's filesystem, so the swap rename is atomic. os.CreateTemp
// creates the file exclusively, so a pre-planted file or symlink is never
// followed or reused. A partial file is removed before an error is returned.
func writeTempBeside(exePath string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(exePath), ".modelsdb-update-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp, 0o755)
	}
	if werr != nil {
		os.Remove(tmp)
		return "", werr
	}
	return tmp, nil
}
