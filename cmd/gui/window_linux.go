//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>

static int glow_w = 0, glow_h = 0;

// glow_target computes the bottom-right anchor for the primary monitor.
static void glow_target(GtkWindow *window, int *tx, int *ty) {
    GdkDisplay *d = gtk_widget_get_display(GTK_WIDGET(window));
    GdkMonitor *m = d ? gdk_display_get_primary_monitor(d) : NULL;
    if (!m && d) m = gdk_display_get_monitor(d, 0);
    GdkRectangle geo = {0, 0, 1280, 720};
    if (m) gdk_monitor_get_geometry(m, &geo);
    *tx = geo.x + geo.width - glow_w - 16;
    *ty = geo.y + geo.height - glow_h - 44; // room above the taskbar
}

// glow_reposition snaps the window back to the corner whenever it's moved, so it
// stays put like a desktop widget (a drag springs it right back). The tolerance
// stops a jitter loop from the WM's own decoration offset.
static gboolean glow_reposition(GtkWidget *widget, GdkEventConfigure *ev, gpointer data) {
    GtkWindow *window = GTK_WINDOW(widget);
    int tx, ty, cx, cy;
    glow_target(window, &tx, &ty);
    gtk_window_get_position(window, &cx, &cy);
    if (abs(cx - tx) > 3 || abs(cy - ty) > 3) {
        gtk_window_move(window, tx, ty);
    }
    return FALSE;
}

// glow_move_bottom_right pins the window to the bottom-right corner and keeps it
// there: on top, on every workspace, and re-snapped if the WM or a drag moves it.
static void glow_move_bottom_right(void *win, int w, int h) {
    if (!win) return;
    glow_w = w;
    glow_h = h;
    GtkWindow *window = GTK_WINDOW(win);
    gtk_widget_realize(GTK_WIDGET(window)); // create the GDK window so the move sticks
    gtk_window_set_gravity(window, GDK_GRAVITY_SOUTH_EAST);
    gtk_window_set_keep_above(window, TRUE); // always-on-top, like the 1.1.1.1 panel
    gtk_window_stick(window);                // visible on all workspaces
    g_signal_connect(window, "configure-event", G_CALLBACK(glow_reposition), NULL);
    int tx, ty;
    glow_target(window, &tx, &ty);
    gtk_window_move(window, tx, ty);
}
*/
import "C"
import "unsafe"

func moveBottomRight(win unsafe.Pointer, w, h int) {
	C.glow_move_bottom_right(win, C.int(w), C.int(h))
}
