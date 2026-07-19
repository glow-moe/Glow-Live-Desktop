// Package pair runs the browser authorize flow: start a pairing, open the link
// for the user to approve on glow.moe, and poll until the push token comes back.
package pair

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 8 * time.Second}

// BaseFrom derives the site base ("https://glow.moe") from the ingest endpoint.
func BaseFrom(endpoint string) string {
	if b := strings.TrimSuffix(endpoint, "/api/live/ingest"); b != endpoint && b != "" {
		return b
	}
	return "https://glow.moe"
}

type startResp struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

type statusResp struct {
	Approved bool   `json:"approved"`
	Expired  bool   `json:"expired"`
	Token    string `json:"token"`
}

// Run performs the whole flow and returns the push token, or an error/timeout.
func Run(base string) (string, error) {
	sr, err := start(base)
	if err != nil {
		return "", err
	}
	fmt.Println("\nAuthorize this device in your browser:")
	fmt.Println("  " + sr.URL)
	if err := openBrowser(sr.URL); err != nil {
		fmt.Println("  (couldn't open the browser automatically - open the link above)")
	}
	fmt.Print("Waiting for approval")

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		fmt.Print(".")
		st, err := poll(base, sr.ID, sr.Secret)
		if err != nil {
			continue
		}
		if st.Expired {
			return "", errors.New("pairing expired, start again")
		}
		if st.Approved && st.Token != "" {
			fmt.Println(" approved ✓")
			return st.Token, nil
		}
	}
	return "", errors.New("timed out waiting for approval")
}

func start(base string) (startResp, error) {
	var sr startResp
	resp, err := httpClient.Post(base+"/api/live/pair/start", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		return sr, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return sr, fmt.Errorf("start returned %s", resp.Status)
	}
	return sr, json.NewDecoder(resp.Body).Decode(&sr)
}

func poll(base, id, secret string) (statusResp, error) {
	var st statusResp
	u := fmt.Sprintf("%s/api/live/pair/status?id=%s&secret=%s", base, url.QueryEscape(id), url.QueryEscape(secret))
	resp, err := httpClient.Get(u)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	return st, json.NewDecoder(resp.Body).Decode(&st)
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
