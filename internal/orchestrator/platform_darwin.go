//go:build darwin

package orchestrator

// League of Legends has a native Mac client with the same Live Client Data
// and LCU APIs as on Windows (lockfile under the .app, see lcu.commonPaths).
const leagueSupported = true
