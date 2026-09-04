//go:build linux && !android

package menubar

/*
#cgo linux pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <webkit/webkit.h>

extern void onKioskExitGesture(void);

static guint32 last_tap_time = 0;
static int tap_count = 0;

static double current_zoom_level = 1.0;

static void on_webview_load_changed(WebKitWebView *web_view, WebKitLoadEvent load_event, gpointer user_data) {
    if (load_event == WEBKIT_LOAD_COMMITTED || load_event == WEBKIT_LOAD_FINISHED) {
        if (current_zoom_level > 0) {
            webkit_web_view_set_zoom_level(web_view, current_zoom_level);
        }
    }
}

static void configure_webkit_view(GtkWidget *widget, gpointer data) {
    if (!widget) return;
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        WebKitWebView *wv = WEBKIT_WEB_VIEW(widget);
        WebKitSettings *s = webkit_web_view_get_settings(wv);
        if (s) {
            webkit_settings_set_zoom_text_only(s, FALSE);
            webkit_settings_set_enable_html5_local_storage(s, TRUE);
            webkit_settings_set_enable_html5_database(s, TRUE);
            webkit_settings_set_allow_universal_access_from_file_urls(s, TRUE);
            webkit_settings_set_allow_file_access_from_file_urls(s, TRUE);
            webkit_settings_set_enable_webaudio(s, TRUE);
            webkit_settings_set_enable_fullscreen(s, TRUE);
            webkit_settings_set_enable_developer_extras(s, TRUE);
            webkit_settings_set_enable_media_stream(s, TRUE);
            webkit_settings_set_enable_mediasource(s, TRUE);
            webkit_settings_set_enable_encrypted_media(s, TRUE);
            webkit_settings_set_allow_modal_dialogs(s, TRUE);
        }
        if (current_zoom_level > 0) {
            webkit_web_view_set_zoom_level(wv, current_zoom_level);
        }
        g_signal_handlers_disconnect_by_func(wv, G_CALLBACK(on_webview_load_changed), NULL);
        g_signal_connect(wv, "load-changed", G_CALLBACK(on_webview_load_changed), NULL);
    }
    for (GtkWidget *child = gtk_widget_get_first_child(widget); child != NULL; child = gtk_widget_get_next_sibling(child)) {
        configure_webkit_view(child, data);
    }
}

static void on_gesture_pressed(GtkGestureClick *gesture, int n_press, double x, double y, gpointer user_data) {
    GtkWidget *widget = gtk_event_controller_get_widget(GTK_EVENT_CONTROLLER(gesture));
    if (!widget) return;
    int width = gtk_widget_get_width(widget);
    int height = gtk_widget_get_height(widget);
    gboolean is_corner = (x <= 80 && y <= 80) // Top-Left
                      || (x >= (width - 80) && y <= 80 && width > 80) // Top-Right
                      || (x <= 80 && y >= (height - 80) && height > 80) // Bottom-Left
                      || (x >= (width - 80) && y >= (height - 80) && width > 80 && height > 80); // Bottom-Right
    if (is_corner) {
        guint32 now = (guint32)(g_get_monotonic_time() / 1000);
        if (now - last_tap_time > 1200) {
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

static void attach_gesture_listener(GtkWidget *widget, gpointer data) {
    if (!widget) return;
    configure_webkit_view(widget, data);
    GtkGesture *click = gtk_gesture_click_new();
    gtk_gesture_single_set_button(GTK_GESTURE_SINGLE(click), GDK_BUTTON_PRIMARY);
    g_signal_connect(click, "pressed", G_CALLBACK(on_gesture_pressed), NULL);
    gtk_widget_add_controller(widget, GTK_EVENT_CONTROLLER(click));
}

static gboolean setup_gestures_idle(gpointer data) {
    GListModel *toplevels = gtk_window_get_toplevels();
    guint n = g_list_model_get_n_items(toplevels);
    for (guint i = 0; i < n; i++) {
        GtkWindow *w = GTK_WINDOW(g_list_model_get_item(toplevels, i));
        if (w) {
            attach_gesture_listener(GTK_WIDGET(w), data);
            g_object_unref(w);
        }
    }
    return G_SOURCE_REMOVE;
}

static void setup_kiosk_gestures(void) {
    g_idle_add(setup_gestures_idle, NULL);
}

static void find_and_set_menubar_visibility(GtkWidget *widget, gpointer data) {
    if (!widget) return;
    gboolean visible = GPOINTER_TO_INT(data);
    const char *name = G_OBJECT_TYPE_NAME(widget);
    if (name && (g_str_has_prefix(name, "GtkPopover") || g_str_has_prefix(name, "GtkMenu") || g_str_has_prefix(name, "GtkHeaderBar"))) {
        gtk_widget_set_visible(widget, visible);
    }
    for (GtkWidget *child = gtk_widget_get_first_child(widget); child != NULL; child = gtk_widget_get_next_sibling(child)) {
        find_and_set_menubar_visibility(child, data);
    }
}

static gboolean set_all_menubars_visible_idle(gpointer data) {
    GListModel *toplevels = gtk_window_get_toplevels();
    guint n = g_list_model_get_n_items(toplevels);
    for (guint i = 0; i < n; i++) {
        GtkWindow *w = GTK_WINDOW(g_list_model_get_item(toplevels, i));
        if (w) {
            find_and_set_menubar_visibility(GTK_WIDGET(w), data);
            g_object_unref(w);
        }
    }
    return G_SOURCE_REMOVE;
}

static void set_menubars_visible(int visible) {
    g_idle_add(set_all_menubars_visible_idle, GINT_TO_POINTER(visible));
}

static gboolean set_native_fullscreen_idle(gpointer data) {
    gboolean fullscreen = GPOINTER_TO_INT(data);
    GListModel *toplevels = gtk_window_get_toplevels();
    guint n = g_list_model_get_n_items(toplevels);
    for (guint i = 0; i < n; i++) {
        GtkWindow *w = GTK_WINDOW(g_list_model_get_item(toplevels, i));
        if (w) {
            if (fullscreen) {
                gtk_window_fullscreen(w);
                find_and_set_menubar_visibility(GTK_WIDGET(w), GINT_TO_POINTER(0));
            } else {
                gtk_window_unfullscreen(w);
                find_and_set_menubar_visibility(GTK_WIDGET(w), GINT_TO_POINTER(1));
            }
            g_object_unref(w);
        }
    }
    return G_SOURCE_REMOVE;
}

static void set_window_fullscreen_native(int fullscreen) {
    g_idle_add(set_native_fullscreen_idle, GINT_TO_POINTER(fullscreen));
}

static gboolean configure_webviews_idle(gpointer data) {
    GListModel *toplevels = gtk_window_get_toplevels();
    guint n = g_list_model_get_n_items(toplevels);
    for (guint i = 0; i < n; i++) {
        GtkWindow *w = GTK_WINDOW(g_list_model_get_item(toplevels, i));
        if (w) {
            configure_webkit_view(GTK_WIDGET(w), data);
            g_object_unref(w);
        }
    }
    return G_SOURCE_REMOVE;
}

static void configure_webviews(void) {
    g_idle_add(configure_webviews_idle, NULL);
}

static void apply_zoom_to_webview(GtkWidget *widget, gpointer data) {
    if (!widget) return;
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        double zoom = *(double *)data;
        webkit_web_view_set_zoom_level(WEBKIT_WEB_VIEW(widget), zoom);
    }
    for (GtkWidget *child = gtk_widget_get_first_child(widget); child != NULL; child = gtk_widget_get_next_sibling(child)) {
        apply_zoom_to_webview(child, data);
    }
}

static gboolean apply_zoom_idle(gpointer data) {
    GListModel *toplevels = gtk_window_get_toplevels();
    guint n = g_list_model_get_n_items(toplevels);
    for (guint i = 0; i < n; i++) {
        GtkWindow *w = GTK_WINDOW(g_list_model_get_item(toplevels, i));
        if (w) {
            apply_zoom_to_webview(GTK_WIDGET(w), data);
            g_object_unref(w);
        }
    }
    g_free(data);
    return G_SOURCE_REMOVE;
}

static void set_webview_zoom_level(double zoom) {
    current_zoom_level = zoom;
    double *zoom_ptr = g_new(double, 1);
    *zoom_ptr = zoom;
    g_idle_add(apply_zoom_idle, zoom_ptr);
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

// ConfigureWebviewSettings ensures WebKitGTK settings for localStorage, indexedDB, and media are enabled.
func ConfigureWebviewSettings() {
	C.configure_webviews()
}

// ApplyWebviewZoom applies the given zoom level to all WebKit WebViews on Linux.
func ApplyWebviewZoom(zoom float64) {
	if zoom > 0 {
		C.set_webview_zoom_level(C.double(zoom))
	}
}

// SetNativeFullscreen directly sets the GTK window fullscreen state on Linux.
func SetNativeFullscreen(fullscreen bool) {
	if fullscreen {
		C.set_window_fullscreen_native(1)
	} else {
		C.set_window_fullscreen_native(0)
	}
}
