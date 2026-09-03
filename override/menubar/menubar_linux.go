//go:build linux && !android

package menubar

/*
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>

extern void onKioskExitGesture(void);

static guint32 last_tap_time = 0;
static int tap_count = 0;

static gboolean on_kiosk_event(GtkWidget *widget, GdkEvent *event, gpointer user_data) {
    if (!event) return FALSE;
    gdouble x = 0, y = 0;
    gboolean is_press = FALSE;
    if (event->type == GDK_BUTTON_PRESS) {
        GdkEventButton *eb = (GdkEventButton *)event;
        if (eb->button == 1) {
            x = eb->x;
            y = eb->y;
            is_press = TRUE;
        }
    }

    if (is_press) {
        gint width = gtk_widget_get_allocated_width(widget);
        if (x >= (width - 80) && y <= 80 && width > 80) {
            guint32 now = gdk_event_get_time(event);
            if (now - last_tap_time > 1000) {
                tap_count = 0;
            }
            last_tap_time = now;
            tap_count++;
            if (tap_count >= 4) {
                tap_count = 0;
                onKioskExitGesture();
            }
        }
    }
    return FALSE; // Propagate event so standard click handling continues
}

static void attach_gesture_listener(GtkWidget *widget, gpointer data) {
    if (!widget) return;
    gtk_widget_add_events(widget, GDK_BUTTON_PRESS_MASK);
    g_signal_connect(G_OBJECT(widget), "event", G_CALLBACK(on_kiosk_event), NULL);
    if (GTK_IS_CONTAINER(widget)) {
        gtk_container_forall(GTK_CONTAINER(widget), attach_gesture_listener, data);
    }
}

static gboolean setup_gestures_idle(gpointer data) {
    GList *toplevels = gtk_window_list_toplevels();
    for (GList *l = toplevels; l != NULL; l = l->next) {
        if (GTK_IS_WINDOW(l->data)) {
            attach_gesture_listener(GTK_WIDGET(l->data), data);
        }
    }
    g_list_free(toplevels);
    return G_SOURCE_REMOVE;
}

static void setup_kiosk_gestures(void) {
    g_idle_add(setup_gestures_idle, NULL);
}

static void find_and_set_menubar_visibility(GtkWidget *widget, gpointer data) {
    gboolean visible = GPOINTER_TO_INT(data);
    if (GTK_IS_MENU_BAR(widget)) {
        if (visible) {
            gtk_widget_show(widget);
        } else {
            gtk_widget_hide(widget);
        }
        return;
    }
    if (GTK_IS_CONTAINER(widget)) {
        gtk_container_forall(GTK_CONTAINER(widget), find_and_set_menubar_visibility, data);
    }
}

static gboolean set_all_menubars_visible_idle(gpointer data) {
    GList *toplevels = gtk_window_list_toplevels();
    for (GList *l = toplevels; l != NULL; l = l->next) {
        if (GTK_IS_WINDOW(l->data)) {
            find_and_set_menubar_visibility(GTK_WIDGET(l->data), data);
        }
    }
    g_list_free(toplevels);
    return G_SOURCE_REMOVE;
}

static void set_menubars_visible(int visible) {
    g_idle_add(set_all_menubars_visible_idle, GINT_TO_POINTER(visible));
}
*/
import "C"

var kioskExitCallback func()

//export onKioskExitGesture
func onKioskExitGesture() {
	if kioskExitCallback != nil {
		kioskExitCallback()
	}
}

// RegisterKioskExitGesture hooks native window events on Linux to detect the 4-tap corner gesture.
func RegisterKioskExitGesture(callback func()) {
	kioskExitCallback = callback
	C.setup_kiosk_gestures()
}

// SetNativeMenubarVisible toggles the visibility of the native GTK menubar on Linux.
func SetNativeMenubarVisible(visible bool) {
	if visible {
		C.set_menubars_visible(1)
	} else {
		C.set_menubars_visible(0)
	}
}
