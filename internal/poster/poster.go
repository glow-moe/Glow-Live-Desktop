// Package poster ships a snapshot to the glow.moe ingest endpoint.
package poster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var client = &http.Client{Timeout: 8 * time.Second}

// Post marshals snap and PUTs it to the ingest endpoint with the bearer token.
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

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ingest returned %s", resp.Status)
	}
	return nil
}
