//go:build windows

package autostart

/*
#cgo LDFLAGS: -ladvapi32
#include <windows.h>

// glow_run_set writes or clears the HKCU Run value that Windows launches at
// sign-in. Per-user, no elevation needed.
static int glow_run_set(const wchar_t *path) {
    HKEY key;
    if (RegCreateKeyExW(HKEY_CURRENT_USER,
        L"Software\\Microsoft\\Windows\\CurrentVersion\\Run",
        0, NULL, 0, KEY_SET_VALUE, NULL, &key, NULL) != ERROR_SUCCESS) {
        return 0;
    }
    LONG rc;
    if (path) {
        rc = RegSetValueExW(key, L"glow L!VE", 0, REG_SZ,
            (const BYTE*)path, (DWORD)((wcslen(path) + 1) * sizeof(wchar_t)));
    } else {
        rc = RegDeleteValueW(key, L"glow L!VE");
        if (rc == ERROR_FILE_NOT_FOUND) rc = ERROR_SUCCESS;
    }
    RegCloseKey(key);
    return rc == ERROR_SUCCESS;
}
*/
import "C"

import (
	"errors"
	"os"
	"unicode/utf16"
	"unsafe"
)

// Set writes or removes the registry Run value.
func Set(on bool) error {
	if !on {
		if C.glow_run_set(nil) == 0 {
			return errors.New("autostart: could not clear the Run value")
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// The path goes in quoted, so spaces in the install dir survive. The
	// --hidden flag makes the boot launch start straight into the tray (a
	// manual double-click has no flag, so it shows the window).
	u := utf16.Encode([]rune("\"" + exe + "\" --hidden"))
	u = append(u, 0)
	if C.glow_run_set((*C.wchar_t)(unsafe.Pointer(&u[0]))) == 0 {
		return errors.New("autostart: could not write the Run value")
	}
	return nil
}
