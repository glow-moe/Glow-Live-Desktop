package fortnite

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Outfit names ("character_hammervice" -> "Coastal Catwoman") from fortnite-api.com,
// for the Discord card. A miss starts the lookup in the background and answers
// "" until it lands, so a presence update never waits on the API. Names are
// kept a day, misses ten minutes.

var (
	nameMu     sync.Mutex
	names      = map[string]nameHit{}
	nameBusy   = map[string]bool{}
	nameBase   = "https://fortnite-api.com/v2/cosmetics/br/"
	nameHTTP   = &http.Client{Timeout: 6 * time.Second}
	cosmeticID = regexp.MustCompile(`^[a-z0-9_]{2,80}$`)
)

type nameHit struct {
	at   time.Time
	name string
}

// OutfitName is the item's name, or "" while it's unknown.
func OutfitName(id string) string {
	id = strings.ToLower(id)
	if !cosmeticID.MatchString(id) {
		return ""
	}
	nameMu.Lock()
	defer nameMu.Unlock()
	hit, ok := names[id]
	ttl := 10 * time.Minute
	if hit.name != "" {
		ttl = 24 * time.Hour
	}
	if (!ok || time.Since(hit.at) >= ttl) && !nameBusy[id] {
		nameBusy[id] = true
		go func() {
			n := fetchName(id)
			nameMu.Lock()
			defer nameMu.Unlock()
			if len(names) > 500 {
				names = map[string]nameHit{}
			}
			names[id] = nameHit{time.Now(), n}
			delete(nameBusy, id)
		}()
	}
	return hit.name
}

func fetchName(id string) string {
	resp, err := nameHTTP.Get(nameBase + id)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var doc struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc) != nil {
		return ""
	}
	n := []rune(doc.Data.Name)
	if len(n) > 40 {
		n = n[:40]
	}
	return string(n)
}
