// Package poster ships a snapshot to the glow.moe ingest endpoint.
package poster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

var client = &http.Client{Timeout: 8 * time.Second}

// ErrOutdated is returned when the server rejects this build as too old to talk
// to it (HTTP 426). The caller should stop and tell the user to update.
var ErrOutdated = errors.New("this app version is no longer supported")

// watched is what the last push's answer said: is anyone looking at this
// member's live view on glow.moe right now? True until told otherwise (an
// older site never says), so nothing slows down by mistake.
var watched atomic.Bool

func init() { watched.Store(true) }

// Watched reports whether glow.moe last said someone is watching.
func Watched() bool { return watched.Load() }

// ErrUnlinked is a push turned down for its key (HTTP 401): the PC was
// unlinked on glow.moe, or the password changed, which unlinks every PC.
var ErrUnlinked = errors.New("this PC is no longer linked")

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

	resp, err := Do(client, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUpgradeRequired { // 426: build too old
		return ErrOutdated
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnlinked
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ingest returned %s", resp.Status)
	}
	var ans struct {
		Watched *bool `json:"watched"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&ans) == nil && ans.Watched != nil {
		watched.Store(*ans.Watched)
	}
	return nil
}

// Version is the build label reported to the server (see SetVersion). Every
// request the server gates on release must carry it, not just the push.
func Version() string { return version }

// Report tells glow.moe about a failure this app hit (POST <base>/api/live/report)
// so it shows on the site's ops page next to the member. Best effort: the answer
// is not read and a failed report is dropped.
func Report(base, token, kind, message, game, osName string) error {
	body, err := json.Marshal(map[string]string{"kind": kind, "message": message, "game": game, "os": osName})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/live/report", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Glow-Live-Version", version)
	resp, err := Do(client, req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
