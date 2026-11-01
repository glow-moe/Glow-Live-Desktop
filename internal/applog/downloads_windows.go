//go:build windows

package applog

import (
	"os"
	"regexp"
	"syscall"
	"unsafe"
)

// downloadsDir is the user's Downloads folder as Windows has it, which can be
// moved off C: (Properties > Location); "" when it can't be read.
func downloadsDir() string {
	var key syscall.Handle
	path, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`)
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_READ, &key) != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	name, _ := syscall.UTF16PtrFromString("{374DE290-123F-4565-9164-39C4925E467B}")
	buf := make([]uint16, 1024)
	n := uint32(len(buf) * 2)
	var typ uint32
	if syscall.RegQueryValueEx(key, name, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return ""
	}
	return expandEnv(syscall.UTF16ToString(buf))
}

var envVarRE = regexp.MustCompile(`%([^%]+)%`)

// expandEnv fills in %USERPROFILE% style variables (the value is often stored
// that way).
func expandEnv(s string) string {
	return envVarRE.ReplaceAllStringFunc(s, func(m string) string {
		if v := os.Getenv(m[1 : len(m)-1]); v != "" {
			return v
		}
		return m
	})
}
