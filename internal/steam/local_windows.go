//go:build windows

package steam

/*
#cgo LDFLAGS: -ladvapi32
#include <windows.h>

// glow_running_appid reads HKCU\Software\Valve\Steam\RunningAppID, which the
// Steam client keeps current for the game it launched. 0 means nothing running,
// and -1 means the key could not be read at all (no Steam on this machine).
static LONG glow_running_appid(void) {
    DWORD value = 0, size = sizeof(value), type = 0;
    HKEY key;
    if (RegOpenKeyExW(HKEY_CURRENT_USER, L"Software\\Valve\\Steam", 0, KEY_READ, &key) != ERROR_SUCCESS) {
        return -1;
    }
    if (RegQueryValueExW(key, L"RunningAppID", NULL, &type, (LPBYTE)&value, &size) != ERROR_SUCCESS
        || type != REG_DWORD) {
        value = 0;
    }
    RegCloseKey(key);
    return (LONG)value;
}
*/
import "C"

func runningAppIDWindows() int { return int(C.glow_running_appid()) }
