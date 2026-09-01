//go:build linux && !android

package menubar

/*
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>

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

// SetNativeMenubarVisible toggles the visibility of the native GTK menubar on Linux.
func SetNativeMenubarVisible(visible bool) {
	if visible {
		C.set_menubars_visible(1)
	} else {
		C.set_menubars_visible(0)
	}
}
