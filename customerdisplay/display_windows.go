//go:build windows

// Package customerdisplay provides the Windows customer display backend.
//
// Implementation uses:
//   - golang.org/x/sys/windows for Win32 monitor enumeration
//   - github.com/wailsapp/go-webview2 for WebView2 rendering
//
// All WebView2 operations are dispatched to a dedicated OS thread that owns a
// COM STA Win32 message loop via runOnUIThread / runOnUIThreadAsync.
// Each public function maps to the platform-agnostic shim in customer_display.go.
package customerdisplay

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// ── Win32 types & constants ───────────────────────────────────────────────

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procShowWindow          = user32.NewProc("ShowWindow")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetPrimaryMonitor   = user32.NewProc("MonitorFromPoint")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type MONITORINFOEX struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
	SzDevice  [32]uint16
}

const (
	MONITOR_DEFAULTTOPRIMARY = 1
	MONITOR_DEFAULTTONEAREST = 2
	MONITORINFOF_PRIMARY     = 1
	SWP_SHOWWINDOW           = 0x0040
	SWP_NOACTIVATE           = 0x0010
	SW_SHOW                  = 5
	SW_HIDE                  = 0

	WM_APP   = 0x8000
	WM_QUIT  = 0x0012
	WM_RUNFN = WM_APP + 1 // custom message: run a func() via PostMessage lParam
)

// ── UI-thread dispatcher ──────────────────────────────────────────────────
//
// WebView2 (go-webview2 / EdgeChromium) must be created, navigated, and
// destroyed from the same OS thread that owns a COM STA message loop.
// We create one dedicated goroutine that is locked to its OS thread and runs
// a Win32 message loop; all WebView2 operations are posted to it.

var (
	uiThreadHWND uintptr
	uiOnce       sync.Once
)

// runOnUIThread posts fn to the UI thread and blocks until it completes.
func runOnUIThread(fn func()) {
	uiOnce.Do(startUIThread)
	done := make(chan struct{})
	wrapper := func() { fn(); close(done) }
	ptr := &wrapper
	user32.NewProc("PostMessageW").Call(
		uiThreadHWND, WM_RUNFN, 0, uintptr(unsafe.Pointer(ptr)),
	)
	<-done
}

// runOnUIThreadAsync posts fn to the UI thread without waiting.
func runOnUIThreadAsync(fn func()) {
	uiOnce.Do(startUIThread)
	wrapper := fn
	ptr := &wrapper
	user32.NewProc("PostMessageW").Call(
		uiThreadHWND, WM_RUNFN, 0, uintptr(unsafe.Pointer(ptr)),
	)
}

func startUIThread() {
	ready := make(chan uintptr, 1)
	go func() {
		runtime.LockOSThread() // bind this goroutine to one OS thread forever

		hInstance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
		// Create a message-only window so PostMessage has a target HWND.
		hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
			0,
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))),
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("CDUIThread"))),
			0,
			0, 0, 0, 0,
			0xFFFFFFFF, // HWND_MESSAGE
			0, hInstance, 0,
		)
		ready <- hwnd

		type MSG struct {
			Hwnd    uintptr
			Message uint32
			WParam  uintptr
			LParam  uintptr
			Time    uint32
			Pt      POINT
		}
		var msg MSG
		procGetMessage := user32.NewProc("GetMessageW")
		procTranslateMessage := user32.NewProc("TranslateMessage")
		procDispatchMessage := user32.NewProc("DispatchMessageW")
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if r == 0 || r == ^uintptr(0) {
				break
			}
			if msg.Message == WM_RUNFN && msg.LParam != 0 {
				fnPtr := (*func())(unsafe.Pointer(msg.LParam))
				(*fnPtr)()
				continue
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	uiThreadHWND = <-ready
}

// ── Singleton state ───────────────────────────────────────────────────────
// All fields are only accessed on the UI thread except via the mu-guarded
// reads in platformReload / platformNavigate.

type displayState struct {
	mu         sync.Mutex
	wv         *edge.Chromium
	hwnd       uintptr
	currentURL string
}

var state displayState

// ── Monitor enumeration (Win32 EnumDisplayMonitors) ───────────────────────

type monitorEnumData struct {
	monitors []MonitorInfo
}

func enumMonitorsCallback(hMonitor, hdcMonitor uintptr, lprcMonitor *RECT, dwData uintptr) uintptr {
	data := (*monitorEnumData)(unsafe.Pointer(dwData))

	var info MONITORINFOEX
	info.CbSize = uint32(unsafe.Sizeof(info))
	procGetMonitorInfo.Call(hMonitor, uintptr(unsafe.Pointer(&info)))

	name := windows.UTF16ToString(info.SzDevice[:])
	isPrimary := (info.DwFlags & MONITORINFOF_PRIMARY) != 0

	idx := len(data.monitors)
	id := fmt.Sprintf("Win-%s-%dx%d-%d-%d-%d",
		name,
		info.RcMonitor.Right-info.RcMonitor.Left,
		info.RcMonitor.Bottom-info.RcMonitor.Top,
		info.RcMonitor.Left,
		info.RcMonitor.Top,
		idx,
	)

	data.monitors = append(data.monitors, MonitorInfo{
		ID:        id,
		Name:      name,
		Width:     int(info.RcMonitor.Right - info.RcMonitor.Left),
		Height:    int(info.RcMonitor.Bottom - info.RcMonitor.Top),
		X:         int(info.RcMonitor.Left),
		Y:         int(info.RcMonitor.Top),
		IsPrimary: isPrimary,
	})
	return 1 // continue enumeration
}

func enumerateMonitors() []MonitorInfo {
	data := &monitorEnumData{}
	cb := windows.NewCallback(func(hMonitor, hdcMonitor uintptr, lprcMonitor *RECT, dwData uintptr) uintptr {
		return enumMonitorsCallback(hMonitor, hdcMonitor, lprcMonitor, dwData)
	})
	procEnumDisplayMonitors.Call(0, 0, cb, uintptr(unsafe.Pointer(data)))
	return data.monitors
}

func findMonitor(monitorID string) *MonitorInfo {
	for _, m := range enumerateMonitors() {
		if m.ID == monitorID {
			mc := m
			return &mc
		}
	}
	return nil
}

// ── WndProc for the WebView2 host window ─────────────────────────────────

var (
	wndProcCallback uintptr
	hostClass       = windows.StringToUTF16Ptr("CDWebViewHost")
)

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	const WM_DESTROY = 0x0002
	if msg == WM_DESTROY {
		state.mu.Lock()
		state.hwnd = 0
		state.wv = nil
		state.currentURL = ""
		state.mu.Unlock()
		return 0
	}
	ret, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, msg, wp, lp)
	return ret
}

// registerWindowClass registers our host HWND class (once).
var registerOnce sync.Once

func registerClass(hInstance uintptr) {
	registerOnce.Do(func() {
		type WNDCLASSEX struct {
			CbSize        uint32
			Style         uint32
			LpfnWndProc   uintptr
			CbClsExtra    int32
			CbWndExtra    int32
			HInstance     uintptr
			HIcon         uintptr
			HCursor       uintptr
			HbrBackground uintptr
			LpszMenuName  *uint16
			LpszClassName *uint16
			HIconSm       uintptr
		}

		wndProcCallback = windows.NewCallback(wndProc)
		wc := WNDCLASSEX{
			LpfnWndProc:   wndProcCallback,
			HInstance:     hInstance,
			LpszClassName: hostClass,
		}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	})
}

// ── WebView2 window management (UI thread only) ───────────────────────────

func openOnMonitor(m *MonitorInfo, url string) {
	if state.hwnd == 0 {
		hInstance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
		registerClass(hInstance)

		hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
			0x00000008, // WS_EX_TOPMOST
			uintptr(unsafe.Pointer(hostClass)),
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Customer Display"))),
			0x80000000, // WS_POPUP
			uintptr(m.X), uintptr(m.Y),
			uintptr(m.Width), uintptr(m.Height),
			0, 0, hInstance, 0,
		)
		if hwnd == 0 {
			return
		}
		state.hwnd = hwnd

		chromium := edge.NewChromium()
		chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)
		if ok := chromium.Embed(state.hwnd); !ok {
			user32.NewProc("DestroyWindow").Call(state.hwnd)
			state.hwnd = 0
			return
		}
		chromium.Resize()
		state.mu.Lock()
		state.wv = chromium
		state.mu.Unlock()
	} else {
		procSetWindowPos.Call(
			state.hwnd, 0,
			uintptr(m.X), uintptr(m.Y),
			uintptr(m.Width), uintptr(m.Height),
			SWP_SHOWWINDOW|SWP_NOACTIVATE,
		)
		state.wv.Resize()
	}

	state.mu.Lock()
	state.currentURL = url
	state.mu.Unlock()
	state.wv.Navigate(url)
	procShowWindow.Call(state.hwnd, SW_SHOW)
}

// ── Platform functions ────────────────────────────────────────────────────

func platformInit() {
	// Enable per-monitor DPI awareness for correct multi-monitor scaling.
	procSetProcessDPIAware.Call()
	uiOnce.Do(startUIThread) // start UI thread eagerly
}

func platformGetMonitors() []MonitorInfo {
	return enumerateMonitors()
}

func platformOpen(monitorID, url string) {
	m := findMonitor(monitorID)
	if m == nil {
		all := enumerateMonitors()
		for i := range all {
			if all[i].IsPrimary {
				m = &all[i]
				break
			}
		}
		if m == nil && len(all) > 0 {
			m = &all[0]
		}
	}
	if m == nil {
		return
	}
	mCopy := *m
	runOnUIThread(func() { openOnMonitor(&mCopy, url) })
}

func platformClose() {
	runOnUIThread(func() {
		if state.hwnd != 0 {
			user32.NewProc("DestroyWindow").Call(state.hwnd)
		}
	})
}

func platformReload() {
	runOnUIThread(func() {
		state.mu.Lock()
		wv := state.wv
		url := state.currentURL
		state.mu.Unlock()
		if wv != nil && url != "" {
			wv.Navigate(url)
		}
	})
}

func platformNavigate(url string) {
	runOnUIThread(func() {
		state.mu.Lock()
		wv := state.wv
		state.mu.Unlock()
		if wv != nil {
			state.mu.Lock()
			state.currentURL = url
			state.mu.Unlock()
			wv.Navigate(url)
		}
	})
}

// ── Identify / Test overlays ──────────────────────────────────────────────

// identifyHTML returns an HTML page showing the monitor number n.
func identifyHTML(n int) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><style>
body{background:rgba(15,23,42,.95);color:#fff;
font-family:system-ui,-apple-system,sans-serif;
display:flex;justify-content:center;align-items:center;
height:100vh;margin:0;overflow:hidden;}
.n{font-size:240px;font-weight:800;
background:linear-gradient(135deg,#a78bfa,#8b5cf6);
-webkit-background-clip:text;-webkit-text-fill-color:transparent;
filter:drop-shadow(0 10px 20px rgba(139,92,246,.3));}
</style></head><body><div class="n">%d</div></body></html>`, n)
}

// testHTML is the static test page content.
const testHTML = `<!DOCTYPE html><html><head><style>
body{background:#0f172a;color:#fff;
font-family:system-ui,-apple-system,sans-serif;
display:flex;flex-direction:column;justify-content:center;align-items:center;
height:100vh;margin:0;overflow:hidden;text-align:center;}
h1{font-size:48px;font-weight:800;letter-spacing:.1em;margin-bottom:24px;
background:linear-gradient(135deg,#a78bfa,#8b5cf6);
-webkit-background-clip:text;-webkit-text-fill-color:transparent;}
p{font-size:20px;color:#94a3b8;max-width:600px;line-height:1.6;}
.line{width:80px;height:4px;background:#8b5cf6;margin:32px auto;border-radius:2px;}
</style></head><body>
<h1>CUSTOMER DISPLAY</h1>
<div class="line"></div>
<p>If you can see this screen,<br>the monitor selection is correct.</p>
<p style="font-size:14px;color:#64748b;margin-top:40px;">This window will close automatically.</p>
</body></html>`

// showHTMLWindow must be called from the UI thread.
func showHTMLWindow(m MonitorInfo, html string, duration time.Duration) {
	hInstance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	registerClass(hInstance)

	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
		0x00000008, // WS_EX_TOPMOST
		uintptr(unsafe.Pointer(hostClass)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("CD Overlay"))),
		0x80000000, // WS_POPUP
		uintptr(m.X), uintptr(m.Y),
		uintptr(m.Width), uintptr(m.Height),
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return
	}

	chromium := edge.NewChromium()
	if ok := chromium.Embed(hwnd); !ok {
		user32.NewProc("DestroyWindow").Call(hwnd)
		return
	}
	chromium.Resize()

	chromium.NavigateToString(html)
	procShowWindow.Call(hwnd, SW_SHOW)

	time.AfterFunc(duration, func() {
		runOnUIThreadAsync(func() {
			user32.NewProc("DestroyWindow").Call(hwnd)
		})
	})
}

func platformIdentify() {
	monitors := enumerateMonitors()
	for i, m := range monitors {
		m := m
		n := i + 1
		go showHTMLWindow(m, identifyHTML(n), 3*time.Second)
	}
}

func platformTest(monitorID string) {
	m := findMonitor(monitorID)
	if m == nil {
		all := enumerateMonitors()
		if len(all) > 0 {
			m = &all[0]
		}
	}
	if m == nil {
		return
	}
	go showHTMLWindow(*m, testHTML, 3*time.Second)
}
