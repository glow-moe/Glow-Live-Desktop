// Package poster ships a snapshot to the glow.moe ingest endpoint.
package poster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var client = &http.Client{Timeout: 8 * time.Second}

// ErrOutdated is returned when the server rejects this build as too old to talk
// to it (HTTP 426). The caller should stop and tell the user to update.
var ErrOutdated = errors.New("this app version is no longer supported")

// version is the build label, reported to the server on every push so it can
// turn away builds that are too old. Set once at startup via SetVersion.
var version = "dev"

// SetVersion records the build label sent on each push (called once at startup).
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

// Post marshals snap and POSTs it to the ingest endpoint with the bearer token.
// delaySec travels as a header so the server can buffer for stream-snipe safety.
func Post(endpoint, token string, delaySec int, snap any) error {
	body, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Glow-Delay", strconv.Itoa(delaySec))
	req.Header.Set("X-Glow-Live-Version", version)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUpgradeRequired { // 426: build too old
		return ErrOutdated
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ingest returned %s", resp.Status)
	}
	return nil
}
