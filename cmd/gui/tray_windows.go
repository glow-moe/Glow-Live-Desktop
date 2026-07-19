//go:build windows

package main

/*
#cgo LDFLAGS: -lshell32 -luser32
#include <windows.h>
#include <shellapi.h>
#include <string.h>

#define GLOW_TRAY_MSG (WM_APP + 1)
#define GLOW_ID_OPEN  0xF001
#define GLOW_ID_QUIT  0xF002

// Original webview window procedure, so unhandled messages still reach it.
static WNDPROC g_orig = NULL;
// Show the "still here" balloon only the first time we hide to tray.
static int g_balloonShown = 0;

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
    nid.hIcon = LoadIconW(NULL, (LPCWSTR)IDI_APPLICATION);
    lstrcpynW(nid.szTip, L"glow L!VE", 128);
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

static LRESULT CALLBACK glow_wndproc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
    switch (msg) {
    case WM_CLOSE:
        // Hide to tray instead of quitting.
        ShowWindow(hwnd, SW_HIDE);
        glow_add_tray(hwnd, g_balloonShown ? 0 : 1);
        g_balloonShown = 1;
        return 0;
    case GLOW_TRAY_MSG:
        if (LOWORD(lp) == WM_RBUTTONUP || LOWORD(lp) == WM_CONTEXTMENU) {
            POINT pt;
            GetCursorPos(&pt);
            HMENU menu = CreatePopupMenu();
            AppendMenuW(menu, MF_STRING, GLOW_ID_OPEN, L"Open glow L!VE");
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

// glow_enable_tray subclasses the webview window so closing hides to the tray.
static void glow_enable_tray(void *win) {
    if (!win || g_orig) return;
    HWND hwnd = (HWND)win;
    g_orig = (WNDPROC)SetWindowLongPtrW(hwnd, GWLP_WNDPROC, (LONG_PTR)glow_wndproc);
}
*/
import "C"
import "unsafe"

// enableCloseToTray makes the window's close button hide to the system tray
// (with a one-time "still running" balloon) instead of quitting. Right-click
// the tray icon to actually quit.
func enableCloseToTray(win unsafe.Pointer) {
	C.glow_enable_tray(win)
}
