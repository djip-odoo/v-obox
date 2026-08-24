#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <stdlib.h>
#include <gdk/gdkkeysyms.h>

extern void goOnMonitorAdded();
extern void goOnMonitorRemoved();

static GtkWidget *customer_window = NULL;
static GtkWidget *customer_webview = NULL;
static char *current_customer_url = NULL;

static void on_customer_window_destroy(GtkWidget *widget, gpointer user_data) {
    customer_window = NULL;
    customer_webview = NULL;
    g_free(current_customer_url);
    current_customer_url = NULL;
}

struct OpenArgs {
    char *monitor_id;
    char *url;
};

static gboolean open_customer_display_idle(gpointer user_data) {
    struct OpenArgs *args = (struct OpenArgs*)user_data;
    GdkDisplay *display = gdk_display_get_default();
    if (!display) {
        g_free(args->monitor_id);
        g_free(args->url);
        g_free(args);
        return FALSE;
    }
    
    int n = gdk_display_get_n_monitors(display);
    int target_idx = -1;
    GdkMonitor *target_monitor = NULL;
    GdkRectangle rect;
    
    for (int i = 0; i < n; i++) {
        GdkMonitor *monitor = gdk_display_get_monitor(display, i);
        const char *model = gdk_monitor_get_model(monitor);
        const char *mfr = gdk_monitor_get_manufacturer(monitor);
        gdk_monitor_get_geometry(monitor, &rect);
        if (!model) model = "Unknown Display";
        if (!mfr) mfr = "Unknown";
        
        char *id = g_strdup_printf("%s-%s-%dx%d-%d-%d-%d", mfr, model, rect.width, rect.height, rect.x, rect.y, i);
        if (g_strcmp0(id, args->monitor_id) == 0) {
            target_idx = i;
            target_monitor = monitor;
            g_free(id);
            break;
        }
        g_free(id);
    }
    
    if (target_idx == -1) {
        if (n > 0) {
            target_idx = 0;
            target_monitor = gdk_display_get_monitor(display, 0);
        } else {
            g_free(args->monitor_id);
            g_free(args->url);
            g_free(args);
            return FALSE;
        }
    }
    
    gdk_monitor_get_geometry(target_monitor, &rect);
    
    if (customer_window) {
        gtk_window_move(GTK_WINDOW(customer_window), rect.x, rect.y);
        gtk_window_resize(GTK_WINDOW(customer_window), rect.width, rect.height);
        
        if (args->url && g_strcmp0(current_customer_url, args->url) != 0) {
            g_free(current_customer_url);
            current_customer_url = g_strdup(args->url);
            webkit_web_view_load_uri(WEBKIT_WEB_VIEW(customer_webview), current_customer_url);
        }
    } else {
        customer_window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
        gtk_window_set_decorated(GTK_WINDOW(customer_window), FALSE);
        gtk_window_set_skip_taskbar_hint(GTK_WINDOW(customer_window), TRUE);
        gtk_window_move(GTK_WINDOW(customer_window), rect.x, rect.y);
        gtk_window_resize(GTK_WINDOW(customer_window), rect.width, rect.height);
        gtk_window_set_keep_above(GTK_WINDOW(customer_window), TRUE);
        
        customer_webview = webkit_web_view_new();
        gtk_container_add(GTK_CONTAINER(customer_window), customer_webview);
        
        g_free(current_customer_url);
        current_customer_url = g_strdup(args->url);
        webkit_web_view_load_uri(WEBKIT_WEB_VIEW(customer_webview), current_customer_url);
        
        g_signal_connect(customer_window, "destroy", G_CALLBACK(on_customer_window_destroy), NULL);
        
        gtk_widget_show_all(customer_window);
        gtk_window_fullscreen(GTK_WINDOW(customer_window));
    }
    
    g_free(args->monitor_id);
    g_free(args->url);
    g_free(args);
    return FALSE;
}

void open_customer_display_c(const char* monitor_id, const char* url) {
    struct OpenArgs *args = malloc(sizeof(struct OpenArgs));
    args->monitor_id = g_strdup(monitor_id);
    args->url = g_strdup(url);
    g_idle_add(open_customer_display_idle, args);
}

static gboolean close_customer_display_idle(gpointer user_data) {
    if (customer_window) {
        gtk_widget_destroy(customer_window);
    }
    return FALSE;
}

void close_customer_display_c() {
    g_idle_add(close_customer_display_idle, NULL);
}

static gboolean reload_customer_display_idle(gpointer user_data) {
    if (customer_webview) {
        webkit_web_view_reload(WEBKIT_WEB_VIEW(customer_webview));
    }
    return FALSE;
}

void reload_customer_display_c() {
    g_idle_add(reload_customer_display_idle, NULL);
}

static gboolean navigate_customer_display_idle(gpointer user_data) {
    char *url = (char*)user_data;
    if (customer_webview && url) {
        g_free(current_customer_url);
        current_customer_url = g_strdup(url);
        webkit_web_view_load_uri(WEBKIT_WEB_VIEW(customer_webview), current_customer_url);
    }
    g_free(url);
    return FALSE;
}

void navigate_customer_display_c(const char* url) {
    g_idle_add(navigate_customer_display_idle, g_strdup(url));
}

static gboolean close_identify_windows(gpointer data) {
    GPtrArray *windows = (GPtrArray*)data;
    for (guint i = 0; i < windows->len; i++) {
        GtkWidget *win = GTK_WIDGET(g_ptr_array_index(windows, i));
        gtk_widget_destroy(win);
    }
    g_ptr_array_free(windows, TRUE);
    return FALSE;
}

static gboolean identify_monitors_idle(gpointer user_data) {
    GdkDisplay *display = gdk_display_get_default();
    if (!display) return FALSE;
    
    int n = gdk_display_get_n_monitors(display);
    GPtrArray *windows = g_ptr_array_new();
    
    for (int i = 0; i < n; i++) {
        GdkMonitor *monitor = gdk_display_get_monitor(display, i);
        GdkRectangle rect;
        gdk_monitor_get_geometry(monitor, &rect);
        
        GtkWidget *win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
        gtk_window_set_decorated(GTK_WINDOW(win), FALSE);
        gtk_window_set_skip_taskbar_hint(GTK_WINDOW(win), TRUE);
        gtk_window_move(GTK_WINDOW(win), rect.x, rect.y);
        gtk_window_resize(GTK_WINDOW(win), rect.width, rect.height);
        gtk_window_set_keep_above(GTK_WINDOW(win), TRUE);
        
        GtkWidget *webview = webkit_web_view_new();
        gtk_container_add(GTK_CONTAINER(win), webview);
        
        char *html = g_strdup_printf(
            "<!DOCTYPE html>"
            "<html>"
            "<head>"
            "<style>"
            "body {"
            "  background-color: rgba(15, 23, 42, 0.95);"
            "  color: white;"
            "  font-family: system-ui, -apple-system, sans-serif;"
            "  display: flex;"
            "  flex-direction: column;"
            "  justify-content: center;"
            "  align-items: center;"
            "  height: 100vh;"
            "  margin: 0;"
            "  overflow: hidden;"
            "}"
            ".number {"
            "  font-size: 240px;"
            "  font-weight: 800;"
            "  background: linear-gradient(135deg, #a78bfa, #8b5cf6);"
            "  -webkit-background-clip: text;"
            "  -webkit-text-fill-color: transparent;"
            "  filter: drop-shadow(0 10px 20px rgba(139, 92, 246, 0.3));"
            "}"
            "</style>"
            "</head>"
            "<body>"
            "<div class=\"number\">%d</div>"
            "</body>"
            "</html>", i + 1
        );
        
        webkit_web_view_load_html(WEBKIT_WEB_VIEW(webview), html, NULL);
        g_free(html);
        
        g_ptr_array_add(windows, win);
        gtk_widget_show_all(win);
        gtk_window_fullscreen(GTK_WINDOW(win));
    }
    
    g_timeout_add(3000, close_identify_windows, windows);
    return FALSE;
}

void identify_monitors_c() {
    g_idle_add(identify_monitors_idle, NULL);
}

static gboolean test_close_timeout(gpointer data) {
    GtkWidget *win = GTK_WIDGET(data);
    gtk_widget_destroy(win);
    return FALSE;
}

static gboolean on_test_window_key_press(GtkWidget *widget, GdkEventKey *event, gpointer user_data) {
    if (event->keyval == GDK_KEY_Escape) {
        gtk_widget_destroy(widget);
        return TRUE;
    }
    return FALSE;
}

static gboolean open_test_display_idle(gpointer user_data) {
    struct OpenArgs *args = (struct OpenArgs*)user_data;
    GdkDisplay *display = gdk_display_get_default();
    if (!display) {
        g_free(args->monitor_id);
        g_free(args);
        return FALSE;
    }
    
    int n = gdk_display_get_n_monitors(display);
    int target_idx = -1;
    GdkMonitor *target_monitor = NULL;
    GdkRectangle rect;
    
    for (int i = 0; i < n; i++) {
        GdkMonitor *monitor = gdk_display_get_monitor(display, i);
        const char *model = gdk_monitor_get_model(monitor);
        const char *mfr = gdk_monitor_get_manufacturer(monitor);
        gdk_monitor_get_geometry(monitor, &rect);
        if (!model) model = "Unknown Display";
        if (!mfr) mfr = "Unknown";
        
        char *id = g_strdup_printf("%s-%s-%dx%d-%d-%d-%d", mfr, model, rect.width, rect.height, rect.x, rect.y, i);
        if (g_strcmp0(id, args->monitor_id) == 0) {
            target_idx = i;
            target_monitor = monitor;
            g_free(id);
            break;
        }
        g_free(id);
    }
    
    if (target_idx == -1) {
        if (n > 0) {
            target_idx = 0;
            target_monitor = gdk_display_get_monitor(display, 0);
        } else {
            g_free(args->monitor_id);
            g_free(args);
            return FALSE;
        }
    }
    
    gdk_monitor_get_geometry(target_monitor, &rect);
    
    GtkWidget *win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_decorated(GTK_WINDOW(win), FALSE);
    gtk_window_set_skip_taskbar_hint(GTK_WINDOW(win), TRUE);
    gtk_window_move(GTK_WINDOW(win), rect.x, rect.y);
    gtk_window_resize(GTK_WINDOW(win), rect.width, rect.height);
    gtk_window_set_keep_above(GTK_WINDOW(win), TRUE);
    
    GtkWidget *webview = webkit_web_view_new();
    gtk_container_add(GTK_CONTAINER(win), webview);
    
    char *html = g_strdup(
        "<!DOCTYPE html>"
        "<html>"
        "<head>"
        "<style>"
        "body {"
        "  background-color: #0f172a;"
        "  color: white;"
        "  font-family: system-ui, -apple-system, sans-serif;"
        "  display: flex;"
        "  flex-direction: column;"
        "  justify-content: center;"
        "  align-items: center;"
        "  height: 100vh;"
        "  margin: 0;"
        "  overflow: hidden;"
        "  text-align: center;"
        "}"
        "h1 {"
        "  font-size: 48px;"
        "  font-weight: 800;"
        "  letter-spacing: 0.1em;"
        "  margin-bottom: 24px;"
        "  background: linear-gradient(135deg, #a78bfa, #8b5cf6);"
        "  -webkit-background-clip: text;"
        "  -webkit-text-fill-color: transparent;"
        "}"
        "p {"
        "  font-size: 20px;"
        "  color: #94a3b8;"
        "  max-width: 600px;"
        "  line-height: 1.6;"
        "}"
        ".line {"
        "  width: 80px;"
        "  height: 4px;"
        "  background-color: #8b5cf6;"
        "  margin: 32px auto;"
        "  border-radius: 2px;"
        "}"
        "</style>"
        "</head>"
        "<body>"
        "<h1>CUSTOMER DISPLAY</h1>"
        "<div class=\"line\"></div>"
        "<p>If you can see this screen,<br>the monitor selection is correct.</p>"
        "<p style=\"font-size: 14px; color: #64748b; margin-top: 40px;\">Press ESC to close, or it will close automatically.</p>"
        "</body>"
        "</html>"
    );
    
    webkit_web_view_load_html(WEBKIT_WEB_VIEW(webview), html, NULL);
    g_free(html);
    
    g_signal_connect(win, "key-press-event", G_CALLBACK(on_test_window_key_press), NULL);
    
    gtk_widget_show_all(win);
    gtk_window_fullscreen(GTK_WINDOW(win));
    
    g_timeout_add(3000, test_close_timeout, win);
    
    g_free(args->monitor_id);
    g_free(args);
    return FALSE;
}

void open_test_display_c(const char* monitor_id) {
    struct OpenArgs *args = malloc(sizeof(struct OpenArgs));
    args->monitor_id = g_strdup(monitor_id);
    args->url = NULL;
    g_idle_add(open_test_display_idle, args);
}

static void on_monitor_added(GdkDisplay *display, GdkMonitor *monitor, gpointer user_data) {
    goOnMonitorAdded();
}

static void on_monitor_removed(GdkDisplay *display, GdkMonitor *monitor, gpointer user_data) {
    goOnMonitorRemoved();
}

static gboolean setup_monitor_signals_idle(gpointer user_data) {
    GdkDisplay *display = gdk_display_get_default();
    if (display) {
        g_signal_connect(display, "monitor-added", G_CALLBACK(on_monitor_added), NULL);
        g_signal_connect(display, "monitor-removed", G_CALLBACK(on_monitor_removed), NULL);
    }
    return FALSE;
}

void setup_monitor_signals_c() {
    g_idle_add(setup_monitor_signals_idle, NULL);
}

char* get_monitors_json_c() {
    GdkDisplay *display = gdk_display_get_default();
    if (!display) return g_strdup("[]");
    
    GdkMonitor *primary_monitor = gdk_display_get_primary_monitor(display);
    int n = gdk_display_get_n_monitors(display);
    GString *json = g_string_new("[");
    
    for (int i = 0; i < n; i++) {
        GdkMonitor *monitor = gdk_display_get_monitor(display, i);
        GdkRectangle rect;
        gdk_monitor_get_geometry(monitor, &rect);
        
        const char *model = gdk_monitor_get_model(monitor);
        const char *mfr = gdk_monitor_get_manufacturer(monitor);
        gboolean is_primary = (monitor == primary_monitor);
        
        if (!model) model = "Unknown Display";
        if (!mfr) mfr = "Unknown";
        
        if (i > 0) g_string_append(json, ",");
        
        g_string_append_printf(json, 
            "{\"id\":\"%s-%s-%dx%d-%d-%d-%d\",\"name\":\"%s %s\",\"width\":%d,\"height\":%d,\"x\":%d,\"y\":%d,\"isPrimary\":%s}",
            mfr, model, rect.width, rect.height, rect.x, rect.y, i,
            mfr, model,
            rect.width, rect.height,
            rect.x, rect.y,
            is_primary ? "true" : "false"
        );
    }
    
    g_string_append(json, "]");
    char *result = g_strdup(json->str);
    g_string_free(json, TRUE);
    return result;
}
