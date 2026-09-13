//go:build windows

package main

/*
#cgo LDFLAGS: -lshell32 -luser32 -lgdi32
#include <windows.h>
#include <shellapi.h>
#include <string.h>

#define GLOW_TRAY_MSG (WM_APP + 1)
#define GLOW_ID_OPEN    0xF001
#define GLOW_ID_QUIT    0xF002
#define GLOW_ID_PROFILE 0xF003
#define GLOW_ID_STATUS  0xF004

extern void glowTrayProfile();

// Tray status line (menu + tooltip) and whether the alert icon is showing.
static wchar_t g_status[128] = L"glow L!VE";
static int g_alert = 0;
static int g_hasProfile = 0;
static HWND g_trayHwnd = NULL;

// Original webview window procedure, so unhandled messages still reach it.
static WNDPROC g_orig = NULL;
// Show the "still here" balloon only the first time we hide to tray.
static int g_balloonShown = 0;
// The app icon, loaded once from the embedded resource (id 1 in icon.rc).
static HICON g_icon = NULL;
// Corner-park target; the window springs back here if the user drags it.
static int g_pinX = 0, g_pinY = 0, g_pinned = 0;

// glow_app_icon loads (once) the .exe's embedded icon so the titlebar, taskbar
// and tray all show the glow mark instead of the generic Windows icon.
static HICON g_alertIcon = NULL;

static HICON glow_app_icon(void) {
    if (!g_icon) {
        HINSTANCE hInst = GetModuleHandleW(NULL);
        g_icon = (HICON)LoadImageW(hInst, MAKEINTRESOURCEW(1), IMAGE_ICON,
            0, 0, LR_DEFAULTSIZE | LR_SHARED);
    }
    return g_icon;
}

// glow_alert_icon draws a red dot over the app icon (bottom-right quarter) so
// the tray shows at a glance that glow.moe is not receiving anything.
static HICON glow_alert_icon(void) {
    if (g_alertIcon) return g_alertIcon;
    HICON base = glow_app_icon();
    if (!base) return NULL;
    int sz = GetSystemMetrics(SM_CXSMICON);
    if (sz < 16) sz = 16;
    HDC screen = GetDC(NULL);
    HDC dc = CreateCompatibleDC(screen);
    BITMAPINFO bi;
    memset(&bi, 0, sizeof(bi));
    bi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bi.bmiHeader.biWidth = sz;
    bi.bmiHeader.biHeight = -sz;
    bi.bmiHeader.biPlanes = 1;
    bi.bmiHeader.biBitCount = 32;
    void *bits = NULL;
    HBITMAP color = CreateDIBSection(dc, &bi, DIB_RGB_COLORS, &bits, NULL, 0);
    HBITMAP old = (HBITMAP)SelectObject(dc, color);
    DrawIconEx(dc, 0, 0, base, sz, sz, 0, NULL, DI_NORMAL);
    // Red dot with a thin white rim, drawn straight into the 32-bit surface so
    // the alpha channel stays intact (GDI pens would zero it).
    unsigned int *px = (unsigned int *)bits;
    double cx = sz * 0.74, cy = sz * 0.74, r = sz * 0.24;
    for (int y = 0; y < sz; y++) {
        for (int x = 0; x < sz; x++) {
            double dx = x + 0.5 - cx, dy = y + 0.5 - cy;
            double d = dx * dx + dy * dy;
            if (d <= r * r) px[y * sz + x] = 0xFFFF3B5C;            // red
            else if (d <= (r + 1.2) * (r + 1.2)) px[y * sz + x] = 0xFFFFFFFF; // rim
        }
    }
    SelectObject(dc, old);
    HBITMAP mask = CreateBitmap(sz, sz, 1, 1, NULL);
    ICONINFO ii;
    memset(&ii, 0, sizeof(ii));
    ii.fIcon = TRUE;
    ii.hbmColor = color;
    ii.hbmMask = mask;
    g_alertIcon = CreateIconIndirect(&ii);
    DeleteObject(mask);
    DeleteObject(color);
    DeleteDC(dc);
    ReleaseDC(NULL, screen);
    return g_alertIcon;
}

static HICON glow_tray_icon(void) {
    HICON ic = g_alert ? glow_alert_icon() : NULL;
    if (!ic) ic = glow_app_icon();
    return ic;
}

static void glow_fill(NOTIFYICONDATAW *nid, HWND hwnd) {
    memset(nid, 0, sizeof(*nid));
    nid->cbSize = sizeof(*nid);
    nid->hWnd = hwnd;
    nid->uID = 1;
}

// Add the tray icon; if balloon is non-zero, also pop a "still running" notice.
static void glow_add_tray(HWND hwnd, int balloon) {
    NOTIFYICONDATAW nid;
    glow_fill(&nid, hwnd);
    nid.uFlags = NIF_ICON | NIF_MESSAGE | NIF_TIP;
    nid.uCallbackMessage = GLOW_TRAY_MSG;
    nid.hIcon = glow_tray_icon();
    if (!nid.hIcon) nid.hIcon = LoadIconW(NULL, (LPCWSTR)IDI_APPLICATION);
    lstrcpynW(nid.szTip, g_status, 128);
    g_trayHwnd = hwnd;
    Shell_NotifyIconW(NIM_ADD, &nid);
    if (balloon) {
        nid.uFlags = NIF_INFO;
        nid.dwInfoFlags = NIIF_INFO;
        lstrcpynW(nid.szInfoTitle, L"glow L!VE", 64);
        lstrcpynW(nid.szInfo,
            L"Still running here. Right-click the tray icon to quit.", 256);
        Shell_NotifyIconW(NIM_MODIFY, &nid);
    }
}

static void glow_del_tray(HWND hwnd) {
    NOTIFYICONDATAW nid;
    glow_fill(&nid, hwnd);
    Shell_NotifyIconW(NIM_DELETE, &nid);
}

static void glow_restore(HWND hwnd) {
    ShowWindow(hwnd, SW_SHOW);
    ShowWindow(hwnd, SW_RESTORE);
    SetForegroundWindow(hwnd);
}

// glow_hide_to_tray tucks the window into the system tray and pops the one-time
// "still running here" balloon. Shared by the close button and the auto-hide
// that fires once the collector starts pushing (see glow_hide / hideToTray).
static void glow_hide_to_tray(HWND hwnd) {
    if (!IsWindowVisible(hwnd)) return; // already in the tray
    ShowWindow(hwnd, SW_HIDE);
    glow_add_tray(hwnd, g_balloonShown ? 0 : 1);
    g_balloonShown = 1;
}

// glow_hide is the void* entry point Go calls to auto-hide to the tray.
static void glow_hide(void *win) {
    if (win) glow_hide_to_tray((HWND)win);
}

static LRESULT CALLBACK glow_wndproc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
    switch (msg) {
    case WM_CLOSE:
        // Hide to tray instead of quitting.
        glow_hide_to_tray(hwnd);
        return 0;
    case WM_EXITSIZEMOVE:
        // Dragged the widget away? Spring it back to the pinned corner.
        if (g_pinned) {
            SetWindowPos(hwnd, HWND_TOPMOST, g_pinX, g_pinY, 0, 0,
                SWP_NOSIZE | SWP_NOACTIVATE);
        }
        break;
    case GLOW_TRAY_MSG:
        if (LOWORD(lp) == WM_RBUTTONUP || LOWORD(lp) == WM_CONTEXTMENU) {
            POINT pt;
            GetCursorPos(&pt);
            HMENU menu = CreatePopupMenu();
            AppendMenuW(menu, MF_STRING | MF_GRAYED, GLOW_ID_STATUS, g_status);
            AppendMenuW(menu, MF_SEPARATOR, 0, NULL);
            AppendMenuW(menu, MF_STRING, GLOW_ID_OPEN, L"Open glow L!VE");
            AppendMenuW(menu, MF_STRING | (g_hasProfile ? 0 : MF_GRAYED), GLOW_ID_PROFILE, L"Open my profile");
            AppendMenuW(menu, MF_SEPARATOR, 0, NULL);
            AppendMenuW(menu, MF_STRING, GLOW_ID_QUIT, L"Quit");
            // Required so the menu dismisses on click-away.
            SetForegroundWindow(hwnd);
            TrackPopupMenu(menu, TPM_RIGHTBUTTON, pt.x, pt.y, 0, hwnd, NULL);
            DestroyMenu(menu);
        } else if (LOWORD(lp) == WM_LBUTTONUP || LOWORD(lp) == WM_LBUTTONDBLCLK) {
            glow_restore(hwnd);
        }
        return 0;
    case WM_COMMAND:
        if (LOWORD(wp) == GLOW_ID_OPEN) {
            glow_restore(hwnd);
            return 0;
        }
        if (LOWORD(wp) == GLOW_ID_PROFILE) {
            glowTrayProfile();
            return 0;
        }
        if (LOWORD(wp) == GLOW_ID_QUIT) {
            glow_del_tray(hwnd);
            DestroyWindow(hwnd); // real close: skips WM_CLOSE, ends the loop
            return 0;
        }
        break;
    case WM_DESTROY:
        glow_del_tray(hwnd); // make sure the icon never lingers
        break;
    }
    return CallWindowProcW(g_orig, hwnd, msg, wp, lp);
}

// glow_tray_update sets the status line (menu + tooltip), the profile item's
// availability and the alert icon. Called on the GUI thread.
static void glow_tray_update(const wchar_t *line, int alert, int hasProfile) {
    lstrcpynW(g_status, line, 128);
    g_alert = alert;
    g_hasProfile = hasProfile;
    if (!g_trayHwnd) return;
    NOTIFYICONDATAW nid;
    glow_fill(&nid, g_trayHwnd);
    nid.uFlags = NIF_ICON | NIF_TIP;
    nid.hIcon = glow_tray_icon();
    lstrcpynW(nid.szTip, g_status, 128);
    Shell_NotifyIconW(NIM_MODIFY, &nid);
}

// glow_enable_tray subclasses the webview window so closing hides to the tray,
// and gives the window the embedded glow icon (titlebar + Alt-Tab + taskbar).
static void glow_enable_tray(void *win) {
    if (!win || g_orig) return;
    HWND hwnd = (HWND)win;
    HICON ic = glow_app_icon();
    if (ic) {
        SendMessageW(hwnd, WM_SETICON, ICON_SMALL, (LPARAM)ic);
        SendMessageW(hwnd, WM_SETICON, ICON_BIG, (LPARAM)ic);
    }
    g_orig = (WNDPROC)SetWindowLongPtrW(hwnd, GWLP_WNDPROC, (LONG_PTR)glow_wndproc);
    // Keep the tray icon present the whole time (no balloon), whether the window
    // is open or hidden, so the app is always reachable from the tray.
    glow_add_tray(hwnd, 0);
}

// glow_pin_bottom_right parks the widget in the bottom-right of the work area
// (above the taskbar), always-on-top, and remembers the spot for snap-back.
static void glow_pin_bottom_right(void *win, int w, int h) {
    if (!win) return;
    HWND hwnd = (HWND)win;
    RECT work;
    if (!SystemParametersInfoW(SPI_GETWORKAREA, 0, &work, 0)) return;
    // Sit a bit higher off the corner so the widget clears the system-tray
    // icons / notification-flyout area at the bottom-right.
    int marginX = 16;
    int marginY = 52;
    g_pinX = work.right - w - marginX;
    g_pinY = work.bottom - h - marginY;
    g_pinned = 1;
    SetWindowPos(hwnd, HWND_TOPMOST, g_pinX, g_pinY, 0, 0,
        SWP_NOSIZE | SWP_NOACTIVATE | SWP_SHOWWINDOW);
}
*/
import "C"
import (
	"syscall"
	"unsafe"
)

// enableCloseToTray makes the window's close button hide to the system tray
// (with a one-time "still running" balloon) instead of quitting. Right-click
// the tray icon to actually quit. It also assigns the embedded app icon.
func enableCloseToTray(win unsafe.Pointer) {
	C.glow_enable_tray(win)
}

// pinBottomRight parks the widget in the bottom-right corner, always-on-top,
// with snap-back on drag. Called by moveBottomRight (window_windows.go).
func pinBottomRight(win unsafe.Pointer, w, h int) {
	C.glow_pin_bottom_right(win, C.int(w), C.int(h))
}

// hideToTray tucks the window into the system tray (with the "still running"
// balloon), the same as pressing the close button. Fired automatically once the
// collector starts pushing so the widget doesn't sit open on screen.
func hideToTray(win unsafe.Pointer) {
	C.glow_hide(win)
}

// showWindow brings the window back from the tray; same path the tray menu's
// Open item takes.
func showWindow(win unsafe.Pointer) {
	if win != nil {
		C.glow_restore((C.HWND)(win))
	}
}

// trayUpdate pushes the status line, tooltip and alert state to the tray.
// Runs on the GUI thread (see watchTray).
func trayUpdate(line string, alert bool, hasProfile bool) {
	w, err := syscall.UTF16PtrFromString("glow L!VE · " + line)
	if err != nil {
		return
	}
	a, p := 0, 0
	if alert {
		a = 1
	}
	if hasProfile {
		p = 1
	}
	C.glow_tray_update((*C.wchar_t)(unsafe.Pointer(w)), C.int(a), C.int(p))
}
