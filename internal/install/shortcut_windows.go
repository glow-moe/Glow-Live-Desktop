//go:build windows

package install

/*
#cgo LDFLAGS: -lole32 -luuid
#define COBJMACROS
#include <windows.h>
#include <shlobj.h>
#include <objbase.h>

// glow_make_shortcut writes a .lnk at `lnk` that starts `target` (with its
// icon), the way an installer adds a Start menu entry.
static int glow_make_shortcut(const wchar_t *lnk, const wchar_t *target, const wchar_t *dir, const wchar_t *desc) {
    HRESULT init = CoInitializeEx(NULL, COINIT_APARTMENTTHREADED);
    IShellLinkW *sl = NULL;
    HRESULT hr = CoCreateInstance(&CLSID_ShellLink, NULL, CLSCTX_INPROC_SERVER, &IID_IShellLinkW, (void **)&sl);
    int ok = 0;
    if (SUCCEEDED(hr)) {
        IShellLinkW_SetPath(sl, target);
        IShellLinkW_SetWorkingDirectory(sl, dir);
        IShellLinkW_SetDescription(sl, desc);
        IShellLinkW_SetIconLocation(sl, target, 0);
        IPersistFile *pf = NULL;
        if (SUCCEEDED(IShellLinkW_QueryInterface(sl, &IID_IPersistFile, (void **)&pf))) {
            ok = SUCCEEDED(IPersistFile_Save(pf, lnk, TRUE));
            IPersistFile_Release(pf);
        }
        IShellLinkW_Release(sl);
    }
    if (SUCCEEDED(init)) CoUninitialize();
    return ok;
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// shortcutPath is the Start menu entry for this user.
func shortcutPath() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "glow L!VE.lnk")
}

// shortcutMark says the Start menu entry was made once, so one the person
// deleted on purpose doesn't come back on every start.
func shortcutMark() string { return filepath.Join(installDir(), "shortcut") }

// ensureShortcut adds glow L!VE to the Start menu (it then shows in search
// and the app list), pointing at the launcher. Once per install.
func ensureShortcut() {
	lnk, target := shortcutPath(), Target()
	if lnk == "" || target == "" || exists(shortcutMark()) {
		return
	}
	if !exists(lnk) && !makeShortcut(lnk, target, "glow L!VE: your games, live on glow.moe") {
		return
	}
	_ = os.WriteFile(shortcutMark(), nil, 0o644)
}

func makeShortcut(lnk, target, desc string) bool {
	if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
		return false
	}
	// COM wants the same OS thread from init to uninit.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	l, _ := syscall.UTF16PtrFromString(lnk)
	t, _ := syscall.UTF16PtrFromString(target)
	d, _ := syscall.UTF16PtrFromString(filepath.Dir(target))
	s, _ := syscall.UTF16PtrFromString(desc)
	return C.glow_make_shortcut((*C.wchar_t)(unsafe.Pointer(l)), (*C.wchar_t)(unsafe.Pointer(t)), (*C.wchar_t)(unsafe.Pointer(d)), (*C.wchar_t)(unsafe.Pointer(s))) != 0
}
