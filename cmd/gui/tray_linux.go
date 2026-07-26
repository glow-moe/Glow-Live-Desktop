//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#cgo LDFLAGS: -ldl
#include <gtk/gtk.h>
#include <dlfcn.h>
#include <stdlib.h>

// The tray is a StatusNotifierItem, spoken through libayatana-appindicator,
// which nearly every desktop ships because Discord and Steam need it too. It is
// loaded at RUNTIME with dlopen: no build-time dependency, and a machine
// without the library simply gets no tray icon while everything else works.

typedef void* (*glow_ai_new_t)(const char*, const char*, int, const char*);
typedef void  (*glow_ai_status_t)(void*, int);
typedef void  (*glow_ai_menu_t)(void*, GtkMenu*);

// Implemented on the Go side (tray_linux_cb.go).
extern void glowTrayOpen();
extern void glowTrayQuit();

static void glow_on_open(GtkMenuItem *item, gpointer data) { glowTrayOpen(); }
static void glow_on_quit(GtkMenuItem *item, gpointer data) { glowTrayQuit(); }

// glow_tray_init builds the indicator + menu. icon_dir must hold glow-live.png.
// Returns 1 when the tray is up, 0 when the library is missing.
static int glow_tray_init(const char *icon_dir) {
    void *lib = dlopen("libayatana-appindicator3.so.1", RTLD_LAZY);
    if (!lib) lib = dlopen("libappindicator3.so.1", RTLD_LAZY);
    if (!lib) return 0;
    glow_ai_new_t    ai_new    = (glow_ai_new_t)dlsym(lib, "app_indicator_new_with_path");
    glow_ai_status_t ai_status = (glow_ai_status_t)dlsym(lib, "app_indicator_set_status");
    glow_ai_menu_t   ai_menu   = (glow_ai_menu_t)dlsym(lib, "app_indicator_set_menu");
    if (!ai_new || !ai_status || !ai_menu) return 0;

    GtkWidget *menu = gtk_menu_new();
    GtkWidget *open = gtk_menu_item_new_with_label("Open glow L!VE");
    GtkWidget *quit = gtk_menu_item_new_with_label("Quit");
    g_signal_connect(open, "activate", G_CALLBACK(glow_on_open), NULL);
    g_signal_connect(quit, "activate", G_CALLBACK(glow_on_quit), NULL);
    gtk_menu_shell_append(GTK_MENU_SHELL(menu), open);
    gtk_menu_shell_append(GTK_MENU_SHELL(menu), gtk_separator_menu_item_new());
    gtk_menu_shell_append(GTK_MENU_SHELL(menu), quit);
    gtk_widget_show_all(menu);

    // 0 = APPLICATION_STATUS, 1 = ACTIVE.
    void *ind = ai_new("glow-live", "glow-live", 0, icon_dir);
    if (!ind) return 0;
    ai_status(ind, 1);
    ai_menu(ind, GTK_MENU(menu));
    return 1;
}

static void glow_hide_window(void *win) {
    if (win) gtk_widget_hide(GTK_WIDGET(win));
}

static void glow_show_window(void *win) {
    if (!win) return;
    gtk_widget_show_all(GTK_WIDGET(win));
    gtk_window_present(GTK_WINDOW(win));
}

// The close button hides instead of quitting, same as the Windows build; the
// menu's Quit is the real exit. Only wired when the tray actually came up,
// otherwise close still quits and nothing gets stranded invisible.
static gboolean glow_on_delete(GtkWidget *w, GdkEvent *e, gpointer data) {
    gtk_widget_hide(w);
    return TRUE;
}

static void glow_close_hides(void *win) {
    if (win) g_signal_connect(GTK_WIDGET(win), "delete-event", G_CALLBACK(glow_on_delete), NULL);
}

// glow_set_icon puts our mark on the window itself, so the titlebar, taskbar
// and alt-tab show glow instead of the desktop's stand-in for iconless apps.
static void glow_set_icon(void *win, const char *png) {
    if (!win) return;
    gtk_window_set_icon_from_file(GTK_WINDOW(win), png, NULL);
    gtk_window_set_default_icon_from_file(png, NULL);
}
*/
import "C"

import (
	_ "embed"
	"os"
	"path/filepath"
	"unsafe"
)

//go:embed trayicon.png
var trayIconPNG []byte

// Shared with the callbacks in tray_linux_cb.go; trayTerminate lives in
// main.go because every platform sets it.
var (
	trayWin unsafe.Pointer
	trayUp  bool
)

// enableCloseToTray brings the tray up and, when that works, turns the close
// button into hide. Runs on the GUI thread (called from w.Dispatch).
func enableCloseToTray(win unsafe.Pointer) {
	trayWin = win
	dir, err := iconDir()
	if err != nil {
		return
	}
	cdir := C.CString(dir)
	defer C.free(unsafe.Pointer(cdir))
	cicon := C.CString(filepath.Join(dir, "glow-live.png"))
	defer C.free(unsafe.Pointer(cicon))
	C.glow_set_icon(win, cicon)
	if C.glow_tray_init(cdir) == 1 {
		trayUp = true
		C.glow_close_hides(win)
	}
}

// hideToTray hides the window; the tray icon is the way back. Without a tray
// there is nothing to come back from, so it refuses to strand the window.
func hideToTray(win unsafe.Pointer) {
	if trayUp {
		C.glow_hide_window(win)
	}
}

func showFromTray() {
	if trayWin != nil {
		C.glow_show_window(trayWin)
	}
}

// iconDir writes the embedded icon where the indicator can read it by name.
func iconDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "glow-collector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "glow-live.png")
	if _, err := os.Stat(p); err != nil {
		if err := os.WriteFile(p, trayIconPNG, 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}
