// Package catalog - fetching data from the configured upstream GitHub repo.
//
// One thing is pulled from GitHub: release info for the update check (see
// updatecheck.go). Its URL derives from the `upstream` config, so a fork that
// sets its own owner/repo/branch in config.json checks its OWN releases.
//
// Privacy: requests carry only Go's default User-Agent. We deliberately send NO
// app- or owner-identifying header to GitHub (no "ModelsDB" UA/referer); the
// only identity in the request is the repo path itself, which is functionally
// required.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const remoteHTTPTimeout = 20 * time.Second

func httpGetBytes(url string) ([]byte, error) {
	client := &http.Client{Timeout: remoteHTTPTimeout}
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

// HTTPGetJSON GETs url and decodes the JSON body into out.
func HTTPGetJSON(url string, out interface{}) error {
	b, err := httpGetBytes(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
