package poster

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/applog"
)

// A second address for glow.moe that skips Cloudflare, for when a provider
// cuts connections to Cloudflare (seen on Turkish ISPs: "an existing
// connection was forcibly closed"). glow.moe hands it out in the L!VE
// settings; it's kept on disk so a start with glow.moe already unreachable
// still has it. Only a glow.moe subdomain over https is taken: the push key
// goes there too.

var fallbackRE = regexp.MustCompile(`^https://[a-z0-9-]+\.glow\.moe$`)

// fallbackFor is how long a fallback that worked stays the first choice.
const fallbackFor = 10 * time.Minute

// primaryHost is glow.moe; requests anywhere else never use the fallback.
var primaryHost = "glow.moe"

var (
	fbMu     sync.Mutex
	fallback string
	fbUntil  time.Time
	fbLoaded bool
)

func fallbackFile() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "glow-collector", "fallback")
}

func loadFallback() {
	if fbLoaded {
		return
	}
	fbLoaded = true
	if p := fallbackFile(); p != "" {
		if b, err := os.ReadFile(p); err == nil && fallbackRE.MatchString(strings.TrimSpace(string(b))) {
			fallback = strings.TrimSpace(string(b))
		}
	}
}

// Fallback is the fallback address in use ("" when none).
func Fallback() string {
	fbMu.Lock()
	defer fbMu.Unlock()
	loadFallback()
	return fallback
}

// SetFallback records the fallback address from the L!VE settings ("" clears
// it). Anything but an https glow.moe subdomain is ignored.
func SetFallback(base string) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base != "" && !fallbackRE.MatchString(base) {
		return
	}
	fbMu.Lock()
	defer fbMu.Unlock()
	loadFallback()
	if base == fallback {
		return
	}
	fallback = base
	if p := fallbackFile(); p != "" {
		if base == "" {
			_ = os.Remove(p)
		} else {
			_ = os.WriteFile(p, []byte(base), 0o600)
		}
	}
}

// Unreachable reports whether err means glow.moe couldn't be reached at all
// (no connection, reset, timeout), as opposed to an answer we didn't like.
func Unreachable(err error) bool {
	var ue *url.Error
	return errors.As(err, &ue)
}

// Do sends req, which goes to glow.moe, and tries the fallback address when
// glow.moe can't be reached. A fallback that worked goes first for a while.
// Requests to anywhere else (a dev endpoint) go as they are.
func Do(c *http.Client, req *http.Request) (*http.Response, error) {
	fbMu.Lock()
	loadFallback()
	fb, preferFb := fallback, time.Now().Before(fbUntil)
	fbMu.Unlock()
	if fb == "" || req.URL.Host != primaryHost {
		return c.Do(req)
	}
	order := []string{"", fb}
	if preferFb {
		order = []string{fb, ""}
	}
	var firstErr error
	for i, base := range order {
		r := req
		if i > 0 || base != "" {
			if r = withBase(req, base); r == nil {
				break
			}
		}
		resp, err := c.Do(r)
		if err == nil {
			fbMu.Lock()
			if base != "" {
				if !time.Now().Before(fbUntil) {
					applog.Printf("glow.moe unreachable, using the backup address")
				}
				fbUntil = time.Now().Add(fallbackFor)
			} else {
				fbUntil = time.Time{} // glow.moe answers again
			}
			fbMu.Unlock()
			return resp, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !Unreachable(err) {
			return nil, err
		}
	}
	return nil, firstErr
}

// withBase is a copy of req sent to base ("" = where it was going), with a
// fresh body; nil when the body can't be sent twice.
func withBase(req *http.Request, base string) *http.Request {
	r := req.Clone(req.Context())
	if base != "" {
		u, err := url.Parse(base)
		if err != nil {
			return nil
		}
		r.URL.Scheme, r.URL.Host, r.Host = u.Scheme, u.Host, ""
	}
	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil
		}
		b, err := req.GetBody()
		if err != nil {
			return nil
		}
		r.Body = b
	}
	return r
}
