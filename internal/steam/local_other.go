//go:build !windows

package steam

// The registry is Windows-only; elsewhere runningAppID reads the process table.
// The -1 stands for "unreadable" and is never used off Windows.
func runningAppIDWindows() int { return -1 }
