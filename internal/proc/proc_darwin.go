//go:build darwin

package proc

import (
	"os/exec"
	"strings"
)

// Running reports whether a process with one of these names is running. macOS
// has no /proc and no Toolhelp snapshot; ps -c lists each process's executable
// name, which is what the callers pass ("RobloxPlayer").
func Running(names ...string) bool {
	out, err := exec.Command("/bin/ps", "-axco", "comm=").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		for _, n := range names {
			if strings.EqualFold(name, n) {
				return true
			}
		}
	}
	return false
}
