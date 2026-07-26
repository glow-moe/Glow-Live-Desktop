package steam

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Local detection of the running game, read from the machine itself.
//
// The community endpoint this package otherwise relies on is cached by Steam for
// two minutes (Cache-Control: private, max-age=120), so after switching games it
// keeps naming the previous one. It also publishes nothing at all while the
// account is Invisible. Reading the client's own state sidesteps both: it is
// instant, works offline, and does not care what the account shows to others.
//
// The appid decides WHICH game; the endpoint is still what supplies the game's
// Rich Presence line, and that is only trustworthy while the two agree.

// RunningApp reports the appid of the game Steam is currently running, and its
// name when the local library still holds the manifest.
//
// ok says whether the machine's Steam state could be read at all, which is a
// different question from whether a game is running: ok with an appid of 0 means
// "nothing is running, and I am sure", and that certainty is what lets the card
// clear the moment a game closes. ok is false only when the local state is
// unreadable, and then the caller has nothing but the endpoint to go on.
func RunningApp() (appID int, name string, ok bool) {
	appID, ok = runningAppID()
	if !ok || appID == 0 {
		return 0, "", ok
	}
	return appID, appName(appID), true
}

// runningAppID asks the platform where it keeps the answer: the registry on
// Windows, the launcher's own process arguments elsewhere.
func runningAppID() (int, bool) {
	if runtime.GOOS == "windows" {
		id := runningAppIDWindows()
		if id < 0 {
			return 0, false
		}
		return id, true
	}
	// Steam launches games through a "reaper" helper whose arguments carry the id
	// ("SteamLaunch AppId=238430"), so the process table is the local source.
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	for _, p := range procs {
		if !p.IsDir() || p.Name()[0] < '0' || p.Name()[0] > '9' {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", p.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if id := appIDFromArgs(string(b)); id != 0 {
			return id, true
		}
	}
	return 0, true
}

// appIDFromArgs pulls the id out of a launcher command line. The arguments are
// NUL separated, so the marker and the value can land in separate fields.
func appIDFromArgs(cmdline string) int {
	s := strings.ReplaceAll(cmdline, "\x00", " ")
	const marker = "SteamLaunch AppId="
	i := strings.Index(s, marker)
	if i < 0 {
		return 0
	}
	rest := s[i+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	id, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return id
}

// appName reads the game's name from its install manifest. Games live across
// several library folders, so the roots come from libraryfolders.vdf. Returns an
// empty string when the manifest is gone (the game was moved or uninstalled
// while running), which the caller treats as "name unknown", not "no game".
//
// Answers are remembered because this runs on every poll and a library can sit
// on a spinning disk: a game does not rename itself mid-session.
func appName(appID int) string {
	if n, hit := cachedName(appID); hit {
		return n
	}
	n := readAppName(appID)
	nameMu.Lock()
	nameCache[appID] = n
	nameMu.Unlock()
	return n
}

var (
	nameMu    sync.Mutex
	nameCache = map[int]string{}
)

func cachedName(appID int) (string, bool) {
	nameMu.Lock()
	defer nameMu.Unlock()
	n, ok := nameCache[appID]
	// An empty answer is not cached: the manifest may still be being written
	// while the game starts, and a retry next poll costs nothing.
	return n, ok && n != ""
}

func readAppName(appID int) string {
	file := "appmanifest_" + strconv.Itoa(appID) + ".acf"
	for _, root := range libraryRoots() {
		b, err := os.ReadFile(filepath.Join(root, "steamapps", file))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := splitQuoted(strings.TrimSpace(line))
			if len(f) >= 2 && strings.EqualFold(f[0], "name") {
				return f[1]
			}
		}
	}
	return ""
}

// libraryRoots lists every Steam library on this machine: the install itself
// plus whatever libraryfolders.vdf points at (extra drives, external disks).
func libraryRoots() []string {
	var roots []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			roots = append(roots, p)
		}
	}
	for _, cfg := range loginUsersPaths() {
		base := filepath.Dir(filepath.Dir(cfg)) // …/Steam/config/loginusers.vdf → …/Steam
		add(base)
		b, err := os.ReadFile(filepath.Join(base, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := splitQuoted(strings.TrimSpace(line))
			if len(f) >= 2 && strings.EqualFold(f[0], "path") {
				add(f[1])
			}
		}
	}
	return roots
}
