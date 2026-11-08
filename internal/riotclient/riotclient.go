// Package riotclient talks to the Riot Client's local API (the lockfile under
// %LOCALAPPDATA%\Riot Games\Riot Client\Config): who is signed in and their own
// presence per game. VALORANT and Teamfight Tactics both run through it. Read
// only; friends' presences come in the same list and are dropped by puuid.
package riotclient

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoClient means the Riot Client isn't up (no lockfile, or it doesn't answer).
var ErrNoClient = errors.New("riot client not running")

// Local is one running Riot Client.
type Local struct {
	Port, Password string
}

// Self is the signed-in account.
type Self struct {
	PUUID, Name, Tag string
}

// Presence is one of your own presences, one per Riot game that's open.
// VALORANT fills Private, Teamfight Tactics PackedData (both base64 JSON).
type Presence struct {
	Product    string `json:"product"`
	Private    string `json:"private"`
	PackedData string `json:"packedData"`
}

func lockfilePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Riot Games", "Riot Client", "Config", "lockfile")
}

// One small pool, like the League client's (lcu.go): a fresh transport per call
// leaks sockets until the local API starts refusing.
var client = &http.Client{
	Timeout: 4 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, // #nosec G402 - localhost Riot cert
		MaxIdleConnsPerHost: 2,
		MaxConnsPerHost:     4,
		IdleConnTimeout:     30 * time.Second,
	},
}

// Open reads the lockfile. It doesn't call the client yet.
func Open() (Local, error) {
	lf := lockfilePath()
	if lf == "" {
		return Local{}, ErrNoClient
	}
	b, err := os.ReadFile(lf)
	if err != nil {
		return Local{}, ErrNoClient
	}
	parts := strings.Split(strings.TrimSpace(string(b)), ":")
	if len(parts) < 5 {
		return Local{}, ErrNoClient
	}
	return Local{Port: parts[2], Password: parts[3]}, nil
}

// Get decodes one local API answer, at most max bytes of it.
func (l Local) Get(path string, max int64, out any) error {
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+l.Port+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("riot:"+l.Password)))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, max)).Decode(out)
}

// Self is who's signed in to the client's chat.
func (l Local) Self() (Self, error) {
	var s struct {
		PUUID    string `json:"puuid"`
		GameName string `json:"game_name"`
		GameTag  string `json:"game_tag"`
	}
	if err := l.Get("/chat/v1/session", 64<<10, &s); err != nil || s.PUUID == "" {
		return Self{}, ErrNoClient
	}
	return Self{PUUID: s.PUUID, Name: s.GameName, Tag: s.GameTag}, nil
}

// Own is your own presence for one product ("valorant", "teamfighttactics").
func (l Local) Own(self Self, product string) (Presence, bool) {
	var pr struct {
		Presences []struct {
			PUUID string `json:"puuid"`
			Presence
		} `json:"presences"`
	}
	if err := l.Get("/chat/v4/presences", 4<<20, &pr); err != nil {
		return Presence{}, false
	}
	for _, p := range pr.Presences {
		if p.PUUID == self.PUUID && p.Product == product {
			return p.Presence, true
		}
	}
	return Presence{}, false
}

// Decode turns a presence blob into its JSON; nil when it isn't one.
func Decode(blob string) []byte {
	if blob == "" {
		return nil
	}
	dec, err := base64.StdEncoding.DecodeString(blob)
	if err != nil || !json.Valid(dec) || len(dec) > 64<<10 {
		return nil
	}
	return dec
}
