//go:build !windows

package orchestrator

// League of Legends readers (Live Client Data + LCU) do not work under Wine or
// on Linux at all, so non-Windows builds never look for the client.
const leagueSupported = false
