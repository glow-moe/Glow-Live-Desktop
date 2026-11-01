//go:build windows

package install

/*
#cgo LDFLAGS: -ladvapi32 -luser32
#include <windows.h>

static int glow_reg_sz(const wchar_t *path, const wchar_t *name, const wchar_t *val) {
    HKEY k;
    if (RegCreateKeyExW(HKEY_CURRENT_USER, path, 0, NULL, 0, KEY_SET_VALUE, NULL, &k, NULL) != ERROR_SUCCESS) return 0;
    LONG rc = RegSetValueExW(k, name, 0, REG_SZ, (const BYTE *)val, (DWORD)((wcslen(val) + 1) * sizeof(wchar_t)));
    RegCloseKey(k);
    return rc == ERROR_SUCCESS;
}

static int glow_reg_dword(const wchar_t *path, const wchar_t *name, DWORD val) {
    HKEY k;
    if (RegCreateKeyExW(HKEY_CURRENT_USER, path, 0, NULL, 0, KEY_SET_VALUE, NULL, &k, NULL) != ERROR_SUCCESS) return 0;
    LONG rc = RegSetValueExW(k, name, 0, REG_DWORD, (const BYTE *)&val, sizeof(val));
    RegCloseKey(k);
    return rc == ERROR_SUCCESS;
}

static void glow_reg_delete_key(const wchar_t *path) {
    RegDeleteTreeW(HKEY_CURRENT_USER, path);
    RegDeleteKeyW(HKEY_CURRENT_USER, path);
}

static void glow_reg_delete_value(const wchar_t *path, const wchar_t *name) {
    HKEY k;
    if (RegOpenKeyExW(HKEY_CURRENT_USER, path, 0, KEY_SET_VALUE, &k) != ERROR_SUCCESS) return;
    RegDeleteValueW(k, name);
    RegCloseKey(k);
}

static void glow_message(const wchar_t *text) {
    MessageBoxW(NULL, text, L"glow L!VE", MB_OK | MB_ICONINFORMATION);
}
*/
import "C"

import (
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The app's entry in Settings > Apps > Installed apps (per user, like Discord):
// name, version, publisher, icon, size, and an Uninstall that runs this exe
// with --uninstall.
// The key's name is plain (Explorer's list skips one with a "!" in it); the
// name people see is DisplayName.
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\glow-live`

// oldUninstallKey is the first name, from test builds before release.
const oldUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\glow L!VE`

func w(s string) *C.wchar_t {
	p, _ := syscall.UTF16PtrFromString(s)
	return (*C.wchar_t)(unsafe.Pointer(p))
}

// Register writes (or refreshes) the Installed apps entry for this version.
// The supervised app calls it at start, so the listed version follows updates.
func Register(version string) {
	if !Supervised() || Target() == "" {
		return
	}
	C.glow_reg_delete_key(w(oldUninstallKey))
	k := w(uninstallKey)
	target := Target()
	C.glow_reg_sz(k, w("DisplayName"), w("glow L!VE"))
	C.glow_reg_sz(k, w("DisplayVersion"), w(strings.TrimPrefix(version, "v")))
	C.glow_reg_sz(k, w("Publisher"), w("SplitFlux LLC"))
	C.glow_reg_sz(k, w("DisplayIcon"), w(target+",0"))
	C.glow_reg_sz(k, w("InstallLocation"), w(installDir()))
	C.glow_reg_sz(k, w("UninstallString"), w(`"`+target+`" --uninstall`))
	C.glow_reg_sz(k, w("QuietUninstallString"), w(`"`+target+`" --uninstall --quiet`))
	C.glow_reg_sz(k, w("URLInfoAbout"), w("https://glow.moe"))
	C.glow_reg_dword(k, w("NoModify"), 1)
	C.glow_reg_dword(k, w("NoRepair"), 1)
	C.glow_reg_dword(k, w("EstimatedSize"), C.DWORD(dirSizeKB(installDir())))
	// The install date is the day the "installed" mark was first written.
	mark := filepath.Join(installDir(), "installed")
	if !exists(mark) {
		_ = os.WriteFile(mark, nil, 0o644)
	}
	day := time.Now()
	if fi, err := os.Stat(mark); err == nil {
		day = fi.ModTime()
	}
	C.glow_reg_sz(k, w("InstallDate"), w(day.Format("20060102")))
}

func dirSizeKB(dir string) uint32 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return uint32(n / 1024)
}

// Uninstall is "Uninstall" in Installed apps: it closes the running app,
// removes the autostart entry, the Start menu shortcut, the Installed apps
// entry, the app's settings and log, and the install folder (the folder goes
// a moment after this process exits, since it runs from there). The key on
// glow.moe stays until it's removed from the L!VE page.
func Uninstall(quiet bool) {
	quitRunningCopy()
	run := w(`Software\Microsoft\Windows\CurrentVersion\Run`)
	C.glow_reg_delete_value(run, w("glow L!VE"))
	C.glow_reg_delete_key(w(uninstallKey))
	C.glow_reg_delete_key(w(oldUninstallKey))
	if p := shortcutPath(); p != "" {
		_ = os.Remove(p)
	}
	if base, err := os.UserConfigDir(); err == nil {
		_ = os.RemoveAll(filepath.Join(base, "glow-collector"))
	}
	dir := installDir()
	if dir != "" && dir != "." && strings.HasSuffix(dir, dirName) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if p := filepath.Join(dir, e.Name()); !samePath(p, selfPath()) {
				_ = os.RemoveAll(p)
			}
		}
		// This exe is the last file in the folder: a hidden shell removes
		// the folder once this process has exited.
		cmd := exec.Command("cmd", "/c", "ping -n 3 127.0.0.1 >nul & rmdir /s /q \""+dir+"\"")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
		_ = cmd.Start()
	}
	if !quiet {
		C.glow_message(w("glow L!VE was removed from this PC.\n\nTo stop your profile from showing it, remove this PC on glow.moe > L!VE too."))
	}
}

// quitRunningCopy asks the app that's running (if any) to quit, through the
// local address it leaves in its config folder, and waits for it to go.
func quitRunningCopy() {
	base, err := os.UserConfigDir()
	if err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(base, "glow-collector", "gui.addr"))
	if err != nil {
		return
	}
	addr := strings.TrimSpace(string(b))
	c := &http.Client{Timeout: 2 * time.Second}
	if resp, err := c.Post(addr+"/api/quit", "application/json", strings.NewReader("{}")); err == nil {
		resp.Body.Close()
	} else {
		return // nothing answering: not running
	}
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		resp, err := c.Get(addr + "/api/status")
		if err != nil {
			break
		}
		resp.Body.Close()
	}
	// The launcher ends right after the app does.
	time.Sleep(1500 * time.Millisecond)
}
