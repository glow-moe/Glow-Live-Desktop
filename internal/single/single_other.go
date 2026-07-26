//go:build !linux && !darwin && !windows

package single

// Acquire always succeeds on platforms without a lock implementation.
func Acquire() bool { return true }
