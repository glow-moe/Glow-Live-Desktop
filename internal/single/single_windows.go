//go:build windows

package single

/*
#include <windows.h>

// glow_single_acquire holds a named mutex for the process lifetime. The Local\
// namespace scopes it to this user session, matching the per-user config.
static int glow_single_acquire(void) {
    HANDLE h = CreateMutexW(NULL, TRUE, L"Local\\glow-live-single");
    if (!h) return 1; // could not even ask; better to run than to refuse
    if (GetLastError() == ERROR_ALREADY_EXISTS) return 0;
    return 1;
}
*/
import "C"

// Acquire reports whether this process is the only copy.
func Acquire() bool { return C.glow_single_acquire() == 1 }
