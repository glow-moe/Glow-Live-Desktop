//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

// glow_move_bottom_right parks the window in the screen's bottom-right corner,
// like a little desktop widget. Positioning is a hint the WM may adjust.
static void glow_move_bottom_right(void *win, int w, int h) {
    if (!win) return;
    GtkWindow *window = GTK_WINDOW(win);
    GdkDisplay *d = gtk_widget_get_display(GTK_WIDGET(window));
    if (!d) return;
    GdkMonitor *m = gdk_display_get_primary_monitor(d);
    if (!m) m = gdk_display_get_monitor(d, 0);
    if (!m) return;
    GdkRectangle geo;
    gdk_monitor_get_geometry(m, &geo);
    int x = geo.x + geo.width - w - 16;
    int y = geo.y + geo.height - h - 44; // leave room above the taskbar
    gtk_window_move(window, x, y);
}
*/
import "C"
import "unsafe"

func moveBottomRight(win unsafe.Pointer, w, h int) {
	C.glow_move_bottom_right(win, C.int(w), C.int(h))
}
