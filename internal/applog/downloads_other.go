//go:build !windows

package applog

// downloadsDir is "" off Windows: Export falls back to ~/Downloads.
func downloadsDir() string { return "" }
