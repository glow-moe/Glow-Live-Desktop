// Package config persists the collector's settings (the push token, target
// endpoint, delay and poll cadence) to a small JSON file in the user config dir.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is what the GUI edits and the runner reads.
type Config struct {
	// Token pairs the collector to a glow.moe profile (bearer on every push).
	Token string `json:"token"`
	// Endpoint is the ingest URL. Kept configurable for local testing.
	Endpoint string `json:"endpoint"`
	// DelaySec is a stream-snipe delay applied server-side (0 = live).
	DelaySec int `json:"delaySec"`
	// PollMs is how often the live game is polled, in milliseconds.
	PollMs int `json:"pollMs"`
	// DiscordClientID is the Discord application id for Rich Presence. Empty
	// disables it. One shared glow app id works for everyone.
	DiscordClientID string `json:"discordClientId"`
	// LeaguePath optionally points at the League install dir (for the client
	// lockfile) when it isn't in a standard location.
	LeaguePath string `json:"leaguePath"`
	// AnimePresence mirrors what you're watching (fed by the browser extension
	// through glow.moe) to Discord Rich Presence while no game is running.
	AnimePresence bool `json:"animePresence"`
}

// Default returns sane starting settings (no token yet).
func Default() Config {
	return Config{
		Endpoint:      "https://glow.moe/api/live/ingest",
		DelaySec:      0,
		PollMs:        1500,
		AnimePresence: true,
	}
}

// Normalize clamps values into safe ranges and fills blanks.
func (c *Config) Normalize() {
	d := Default()
	if c.Endpoint == "" {
		c.Endpoint = d.Endpoint
	}
	if c.PollMs < 500 {
		c.PollMs = d.PollMs
	}
	if c.PollMs > 10000 {
		c.PollMs = 10000
	}
	if c.DelaySec < 0 {
		c.DelaySec = 0
	}
	if c.DelaySec > 600 {
		c.DelaySec = 600
	}
}

// Path is the on-disk config location (…/glow-collector/config.json).
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "glow-collector", "config.json"), nil
}

// Load reads the config, returning defaults when the file is absent.
func Load() (Config, error) {
	c := Default()
	p, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	c.Normalize()
	return c, nil
}

// Save writes the config, creating the directory if needed.
func Save(c Config) error {
	c.Normalize()
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}
