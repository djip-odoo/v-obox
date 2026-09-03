//go:build !linux || android

package menubar

// SetNativeMenubarVisible is a no-op fallback on Windows and macOS where Wails menu replacement works natively.
func SetNativeMenubarVisible(visible bool) {
}

// RegisterKioskExitGesture is a no-op fallback on non-Linux platforms.
func RegisterKioskExitGesture(callback func()) {
}

// ConfigureWebviewSettings is a no-op fallback on non-Linux platforms.
func ConfigureWebviewSettings() {
}

// ApplyWebviewZoom is a no-op fallback on non-Linux platforms.
func ApplyWebviewZoom(zoom float64) {
}
