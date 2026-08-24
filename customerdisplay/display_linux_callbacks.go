//go:build linux

// display_linux_callbacks.go — CGo export symbols shared by both
// display_linux.go (webkit2gtk-4.1) and display_linux_webkit40.go (webkit2gtk-4.0).
//
// The //export directive requires an import "C" block in the same file.
// This file uses an empty CGo header; the actual declarations live in
// the per-webkit-version files.
package customerdisplay

/*
// empty — just enough for //export to work
*/
import "C"

// ── C callback functions exported to the C layer ──────────────────────────

//export goOnMonitorAdded
func goOnMonitorAdded() {
	triggerCallback()
}

//export goOnMonitorRemoved
func goOnMonitorRemoved() {
	triggerCallback()
}

