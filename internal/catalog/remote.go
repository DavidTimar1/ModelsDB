// Fetching data from the configured upstream GitHub repo.
//
// One thing is pulled from GitHub: release info for the update check (see
// updatecheck.go). Its URL derives from the `upstream` config, so a fork that
// sets its own owner/repo/branch in config.json checks its OWN releases.
//
// Requests carry the one plain browser User-Agent and no app- or
// owner-identifying header (no "ModelsDB" agent or referer); the only identity
// in the request is the repo path itself, which is functionally required.

package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"modelsdb/internal/shared"
	"net/http"
	"time"
)

const (
	remoteHTTPTimeout = 20 * time.Second
	// remoteMaxBodyBytes bounds what a release listing may weigh.
	remoteMaxBodyBytes = 8 << 20
)

func httpGetBytes(url string) ([]byte, error) {
	client := shared.NewHTTPClient(remoteHTTPTimeout)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > remoteMaxBodyBytes {
		return nil, fmt.Errorf("GET %s: body is larger than the %d byte limit", url, remoteMaxBodyBytes)
	}
	return body, nil
}

// HTTPGetJSON GETs url and decodes the JSON body into out.
func HTTPGetJSON(url string, out interface{}) error {
	b, err := httpGetBytes(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
