//go:build windows

package main

// Native live-activity widget: a small always-on-top window (no browser,
// no cgo, pure Win32 via x/sys). It polls /api/live every second and draws
// the running/idle state plus the top sessions with GDI.
//
// Usage:
//
//	9router widget [--host 127.0.0.1] [--port 20130]
//	  [--bg 0B0E14] [--accent 4ADE80] [--opacity 235] [--password ...]
//
// Drag anywhere to move. Right-click for the close menu.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32  = windows.NewLazySystemDLL("user32.dll")
	modGdi32   = windows.NewLazySystemDLL("gdi32.dll")
	modKernel  = windows.NewLazySystemDLL("kernel32.dll")
	modShell   = windows.NewLazySystemDLL("shell32.dll")
	modGdiplus = windows.NewLazySystemDLL("gdiplus.dll")

	pRegisterClassExW            = modUser32.NewProc("RegisterClassExW")
	pCreateWindowExW             = modUser32.NewProc("CreateWindowExW")
	pDefWindowProcW              = modUser32.NewProc("DefWindowProcW")
	pShowWindow                  = modUser32.NewProc("ShowWindow")
	pUpdateWindow                = modUser32.NewProc("UpdateWindow")
	pGetMessageW                 = modUser32.NewProc("GetMessageW")
	pTranslateMessage            = modUser32.NewProc("TranslateMessage")
	pDispatchMessageW            = modUser32.NewProc("DispatchMessageW")
	pPostQuitMessage             = modUser32.NewProc("PostQuitMessage")
	pSetTimer                    = modUser32.NewProc("SetTimer")
	pKillTimer                   = modUser32.NewProc("KillTimer")
	pInvalidateRect              = modUser32.NewProc("InvalidateRect")
	pSetLayeredWindowAttr        = modUser32.NewProc("SetLayeredWindowAttributes")
	pBeginPaint                  = modUser32.NewProc("BeginPaint")
	pEndPaint                    = modUser32.NewProc("EndPaint")
	pSendMessageW                = modUser32.NewProc("SendMessageW")
	pPostMessageW                = modUser32.NewProc("PostMessageW")
	pCreatePopupMenu             = modUser32.NewProc("CreatePopupMenu")
	pAppendMenuW                 = modUser32.NewProc("AppendMenuW")
	pTrackPopupMenu              = modUser32.NewProc("TrackPopupMenu")
	pDestroyMenu                 = modUser32.NewProc("DestroyMenu")
	pSetForegroundWindow         = modUser32.NewProc("SetForegroundWindow")
	pGetCursorPos                = modUser32.NewProc("GetCursorPos")
	pGetSystemMetrics            = modUser32.NewProc("GetSystemMetrics")
	pCreateSolidBrush            = modGdi32.NewProc("CreateSolidBrush")
	pDeleteObject                = modGdi32.NewProc("DeleteObject")
	pSelectObject                = modGdi32.NewProc("SelectObject")
	pSetTextColor                = modGdi32.NewProc("SetTextColor")
	pSetBkMode                   = modGdi32.NewProc("SetBkMode")
	pTextOutW                    = modGdi32.NewProc("TextOutW")
	pEllipse                     = modGdi32.NewProc("Ellipse")
	pCreateFontW                 = modGdi32.NewProc("CreateFontW")
	pFillRect                    = modUser32.NewProc("FillRect")
	pRoundRect                   = modGdi32.NewProc("RoundRect")
	pGetModuleHandleW            = modKernel.NewProc("GetModuleHandleW")
	pLoadIconW                   = modUser32.NewProc("LoadIconW")
	pIsWindowVisible             = modUser32.NewProc("IsWindowVisible")
	pSetWindowPos                = modUser32.NewProc("SetWindowPos")
	pShellNotifyIconW            = modShell.NewProc("Shell_NotifyIconW")
	pCreateCompatibleDC          = modGdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap      = modGdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC                    = modGdi32.NewProc("DeleteDC")
	pBitBlt                      = modGdi32.NewProc("BitBlt")
	pCreateRoundRectRgn          = modGdi32.NewProc("CreateRoundRectRgn")
	pSetWindowRgn                = modUser32.NewProc("SetWindowRgn")
	pCreateMutexW                = modKernel.NewProc("CreateMutexW")
	pGetLastError                = modKernel.NewProc("GetLastError")
	pCloseHandleW                = modKernel.NewProc("CloseHandle")
	pLoadImageW                  = modUser32.NewProc("LoadImageW")
	pDestroyIcon                 = modUser32.NewProc("DestroyIcon")
	pGdiplusStartup              = modGdiplus.NewProc("GdiplusStartup")
	pGdiplusShutdown             = modGdiplus.NewProc("GdiplusShutdown")
	pGdipCreateBitmapFromFile    = modGdiplus.NewProc("GdipCreateBitmapFromFile")
	pGdipCreateHBITMAPFromBitmap = modGdiplus.NewProc("GdipCreateHBITMAPFromBitmap")
	pGdipDisposeImage            = modGdiplus.NewProc("GdipDisposeImage")
	pGdipGetImageWidth           = modGdiplus.NewProc("GdipGetImageWidth")
	pGdipGetImageHeight          = modGdiplus.NewProc("GdipGetImageHeight")
	pGdipGetFrameDimensionsCount = modGdiplus.NewProc("GdipGetFrameDimensionsCount")
	pGdipGetFrameDimensionsList  = modGdiplus.NewProc("GdipGetFrameDimensionsList")
	pGdipGetFrameCount           = modGdiplus.NewProc("GdipGetFrameCount")
	pGdipSelectActiveFrame       = modGdiplus.NewProc("GdipSelectActiveFrame")
	pStretchBlt                  = modGdi32.NewProc("StretchBlt")
	pCreateIconIndirect          = modUser32.NewProc("CreateIconIndirect")
	pCreateEllipticRgn           = modGdi32.NewProc("CreateEllipticRgn")
	pSelectClipRgn               = modGdi32.NewProc("SelectClipRgn")
	pCreateBitmap                = modGdi32.NewProc("CreateBitmap")
	pGetObjectW                  = modGdi32.NewProc("GetObjectW")
	pGetDC                       = modUser32.NewProc("GetDC")
	pReleaseDC                   = modUser32.NewProc("ReleaseDC")
	pDrawIconEx                  = modUser32.NewProc("DrawIconEx")
	pRegisterWindowMessageW      = modUser32.NewProc("RegisterWindowMessageW")
	pGetWindowRect               = modUser32.NewProc("GetWindowRect")
	pSetProcessDPIAware          = modUser32.NewProc("SetProcessDPIAware")
)

// bitmapDimensions mirrors the BITMAP struct fields we need from GetObjectW.
type bitmapDimensions struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

// GdiplusStartupInput is the GDI+ startup parameter block (version must be 1).
type GdiplusStartupInput struct {
	GdiplusVersion          uint32
	DebugEventCallback      uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs  int32
}

// gdiplusInit initializes GDI+ for loading JPEG icons.
func gdiplusInit() (uintptr, bool) {
	var input GdiplusStartupInput
	input.GdiplusVersion = 1
	var token uintptr
	ret, _, _ := pGdiplusStartup.Call(uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&input)), 0)
	if ret != 0 {
		return 0, false
	}
	return token, true
}

func gdiplusShutdown(token uintptr) {
	pGdiplusShutdown.Call(token)
}

// widgetHIcon is the process icon (panda .ico) stored so paintCompact can
// draw it without reloading. Set once in runWidget.
var widgetHIcon windows.Handle

// widgetIconKeepAlive pins the bitmaps backing widgetHIcon. CreateIconIndirect
// keeps referencing the color+mask bitmaps, so freeing them (or letting the
// GC reuse the memory) corrupts the icon after a long idle and the small
// widget paints blank / loses its icon and buttons. Never delete these.
var widgetIconBitmaps []uintptr

// widgetFetchBusy serializes the 1s poll timer: without it every tick spawns
// a goroutine (each with a 5s HTTP timeout), so a long idle/offline stretch
// piles up overlapping fetches + concurrent InvalidateRect calls from worker
// threads, starving the UI thread until the pill goes blank and clicks die.
var widgetFetchBusy atomic.Bool

// drawWidgetIcon blits the widget's panda icon into the DC, clipped to a
// circle so the photo's square background disappears on the dark widget.
func drawWidgetIcon(hdc windows.Handle, x, y, size int32) bool {
	if widgetHIcon == 0 {
		return false
	}
	rgn, _, _ := pCreateEllipticRgn.Call(uintptr(x), uintptr(y), uintptr(x+size), uintptr(y+size))
	if rgn != 0 {
		_, _, _ = pSelectClipRgn.Call(uintptr(hdc), rgn)
	}
	ret, _, _ := pDrawIconEx.Call(uintptr(hdc), uintptr(x), uintptr(y), uintptr(widgetHIcon), uintptr(size), uintptr(size), 0, 0, 3)
	_, _, _ = pSelectClipRgn.Call(uintptr(hdc), 0)
	if rgn != 0 {
		_, _, _ = pDeleteObject.Call(rgn)
	}
	return ret != 0
}

// orbitXY is a 12-step circle used by the activity orbit animation.
var orbitXY = [12][2]int32{
	{15, 0}, {13, 8}, {8, 13}, {0, 15}, {-8, 13}, {-13, 8},
	{-15, 0}, {-13, -8}, {-8, -13}, {0, -15}, {8, -13}, {13, -8},
}

// paintOrbit draws three dots circling (cx,cy): cute "alive" signal
// while the gateway is running. Frame comes from widgetPulse.
func paintOrbit(mem windows.Handle, cx, cy int32, frame int) {
	brushes := []uint32{0x003DA3E8, 0x00CEBF29, 0x006B7280}
	for k := 0; k < 3; k++ {
		b, _, _ := pCreateSolidBrush.Call(uintptr(brushes[k]))
		if b == 0 {
			continue
		}
		prev, _, _ := pSelectObject.Call(uintptr(mem), b)
		o := orbitXY[(frame+k*4)%12]
		dx, dyy := cx+o[0]-2, cy+o[1]-2
		_, _, _ = pEllipse.Call(uintptr(mem), uintptr(dx), uintptr(dyy), uintptr(dx+5), uintptr(dyy+5))
		_, _, _ = pSelectObject.Call(uintptr(mem), prev)
		_, _, _ = pDeleteObject.Call(b)
	}
}

// loadIconFromImage loads an icon from a JPEG/PNG file using GDI+.
func loadIconFromImage(path string) windows.Handle {
	// Initialize GDI+
	token, ok := gdiplusInit()
	if !ok {
		fmt.Fprintf(os.Stderr, "widget: GDI+ startup failed\n")
		return 0
	}
	defer gdiplusShutdown(token)

	// Convert path to UTF-16
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "widget: UTF16 failed: %v\n", err)
		return 0
	}

	// Create bitmap from file
	var bitmap uintptr
	ret, _, _ := pGdipCreateBitmapFromFile.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&bitmap)),
	)
	if ret != 0 || bitmap == 0 {
		fmt.Fprintf(os.Stderr, "widget: GdipCreateBitmapFromFile failed: ret=%d\n", ret)
		return 0
	}
	defer pGdipDisposeImage.Call(bitmap)

	// Create HBITMAP from bitmap
	var hbm uintptr
	ret, _, _ = pGdipCreateHBITMAPFromBitmap.Call(
		bitmap,
		uintptr(unsafe.Pointer(&hbm)),
		0xFFFFFFFF, // background for alpha pixels
	)
	if ret != 0 || hbm == 0 {
		fmt.Fprintf(os.Stderr, "widget: GdipCreateHBITMAPFromBitmap failed: ret=%d\n", ret)
		return 0
	}
	defer pDeleteObject.Call(hbm)

	// Scale the GDI+ bitmap (original size) down to 32x32 for the icon —
	// CreateIconIndirect expects a color bitmap matching the mask size.
	screen, _, _ := pGetDC.Call(0)
	if screen == 0 {
		return 0
	}
	defer pReleaseDC.Call(0, screen)
	srcDC, _, _ := pCreateCompatibleDC.Call(screen)
	if srcDC == 0 {
		return 0
	}
	defer pDeleteDC.Call(srcDC)
	_, _, _ = pSelectObject.Call(srcDC, hbm)
	dstDC, _, _ := pCreateCompatibleDC.Call(screen)
	if dstDC == 0 {
		return 0
	}
	defer pDeleteDC.Call(dstDC)
	scaled, _, _ := pCreateCompatibleBitmap.Call(screen, 32, 32)
	if scaled == 0 {
		return 0
	}
	// NOTE: no defer DeleteObject(scaled) here — ownership passes to the
	// HICON below and the handle is pinned in widgetIconBitmaps.
	_, _, _ = pSelectObject.Call(dstDC, scaled)
	var bd bitmapDimensions
	pGetObjectW.Call(hbm, unsafe.Sizeof(bd), uintptr(unsafe.Pointer(&bd)))
	sw, sh := bd.bmWidth, bd.bmHeight
	if sw <= 0 || sh <= 0 {
		sw, sh = 32, 32
	}
	_, _, _ = pStretchBlt.Call(dstDC, 0, 0, 32, 32, srcDC, 0, 0, uintptr(sw), uintptr(sh), srccopy)

	// Create a 1bpp all-black mask (fully opaque icon)
	hmask, _, _ := pCreateBitmap.Call(32, 32, 1, 1, 0)
	if hmask == 0 {
		_, _, _ = pDeleteObject.Call(scaled)
		return 0
	}

	// Create HICON from HBITMAP via ICONINFO
	ii := struct {
		fIcon    int32
		xHotspot uint32
		yHotspot uint32
		hbmMask  uintptr
		hbmColor uintptr
	}{1, 0, 0, hmask, scaled}
	hicon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if hicon == 0 {
		fmt.Fprintf(os.Stderr, "widget: CreateIconIndirect failed\n")
		_, _, _ = pDeleteObject.Call(scaled)
		_, _, _ = pDeleteObject.Call(hmask)
		return 0
	}
	// Keep color+mask bitmaps alive for the life of the process: the HICON
	// references them and deleting here corrupts the icon after long idle.
	widgetIconBitmaps = append(widgetIconBitmaps, scaled, hmask)

	return windows.Handle(hicon)
}

const (
	wsPopup          = 0x80000000
	wsVisible        = 0x10000000
	wsExTopmost      = 0x00000008
	wsExLayered      = 0x00080000
	wsExToolwindow   = 0x00000080
	swShowNoActive   = 4
	wmDestroy        = 0x0002
	wmPaint          = 0x000F
	wmTimer          = 0x0113
	wmLButtonDown    = 0x0201
	wmRButtonUp      = 0x0205
	wmCommand        = 0x0111
	wmNCLButtonDwn   = 0x00A1
	wmLButtonUp      = 0x0202
	wmLButtonDbl     = 0x0203
	csDblClks        = 0x0008
	srccopy          = 0x00CC0020
	psSolid          = 0
	swHide           = 0
	swShow           = 5
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpNoSize        = 0x0001
	htCaption        = 2
	lwaAlpha         = 0x00000002
	mfString         = 0x00000000
	tpmRightButton   = 0x0002
	tpmReturnCmd     = 0x0100
	smCxScreen       = 0
	transparentBk    = 1
	errAlreadyExists = 183
	trayMsg          = 0x8001 // WM_APP+1: mouse events from the tray icon
	nimAdd           = 0
	nimModify        = 1
	nimDelete        = 2
	nifMessage       = 0x01
	nifIcon          = 0x02
	nifTip           = 0x04
	idiApp           = 32512
	cmdClose         = 1
	cmdSize          = 2
	cmdShow          = 3
)

// trayIconPath holds the path to a custom tray icon (set via --icon flag).
var trayIconPath string

// trayIconCache holds the decoded tray icon so the 1s tooltip refresh does
// not reload GDI+ (and leak an HICON) every tick.
var trayIconCache windows.Handle

// trayIconIco caches the HICON loaded from a .ico path. The .ico branch used
// to call LoadImageW every 1s tick without DestroyIcon, leaking one HICON
// per second until the process GDI handle table filled (~10k handles in
// ~2.8h) and the widget froze/rendered blank. Cache once, reuse forever.
var trayIconIco windows.Handle

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type wPoint struct{ x, y int32 }

type wMsg struct {
	hwnd    windows.HWND
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      wPoint
}

type wPaintStruct struct {
	hdc         windows.Handle
	fErase      int32
	rcPaint     [4]int32
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type widgetCfg struct {
	host     string
	port     string
	bg       uint32 // COLORREF 0x00BBGGRR
	accent   uint32
	opacity  byte
	password string
}

type widgetState struct {
	mu       sync.Mutex
	online   bool
	authed   bool
	active   int
	streams  int
	waiting  int
	inflight int
	retries  int
	errors   int
	tps      float64
	rps      float64
	sessions []widgetSession
	note     string
}

type widgetSession struct {
	name     string
	step     string
	status   string
	ageSec   int
	provider string
	ttfbMs   int64
}

// notifyIconData mirrors NOTIFYICONDATAW (only the fields we use).
type notifyIconData struct {
	cbSize           uint32
	hWnd             windows.HWND
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     windows.Handle
}

// Dimensions: slim pill (default) and compact meter card with segmented bars.
const (
	smallW, smallH = 220, 48
)

var taskbarCreatedMsg uint32

// widgetPos tracks the current bottom-right anchor so a size toggle keeps the
// widget glued to the right edge and a small margin above the taskbar instead
// of jumping around the screen.
var widgetPos struct {
	mu   sync.Mutex
	x, y int32
}

// widgetPulse is the animation frame for the LIVE dot (advanced by the fast
// timer only while a request is running, mirroring the web ping animation).
var widgetPulse atomic.Int32

var wcfg widgetCfg
var wstate widgetState
var wclient *http.Client

// Pinned for the lifetime of the process: Windows keeps calling the window
// procedure and reading the class name long after CreateWindowEx returns.
// If these were only referenced by locals, the GC could free them and the
// widget would vanish/crash minutes after opening.
var (
	wndProcAddr   = windows.NewCallback(widgetWndProc)
	widgetClsName = mustUTF16("9routerWidget")
	widgetTitle   = mustUTF16("9router Widget")
	widgetFont    = mustUTF16("Segoe UI")
)

func parseHexColor(s string, fallback uint32) uint32 {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return fallback
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return fallback
	}
	// HTML RRGGBB -> COLORREF 0x00BBGGRR
	r := (v >> 16) & 0xFF
	g := (v >> 8) & 0xFF
	b := v & 0xFF
	return uint32(b<<16 | g<<8 | r)
}

func widgetBaseURL() string { return "http://" + wcfg.host + ":" + wcfg.port }

func widgetLogin() {
	body, _ := json.Marshal(map[string]string{"password": wcfg.password})
	resp, err := wclient.Post(widgetBaseURL()+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		wstate.mu.Lock()
		wstate.authed = true
		wstate.mu.Unlock()
	}
}

func widgetFetch() {
	resp, err := wclient.Get(widgetBaseURL() + "/api/live")
	if err != nil {
		wstate.mu.Lock()
		wstate.online = false
		wstate.note = "server mati?"
		wstate.mu.Unlock()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		wstate.mu.Lock()
		wstate.online = true
		wstate.authed = false
		wstate.note = "butuh login: --password"
		wstate.mu.Unlock()
		return
	}
	if resp.StatusCode != http.StatusOK {
		wstate.mu.Lock()
		wstate.online = true
		wstate.note = fmt.Sprintf("http %d", resp.StatusCode)
		wstate.mu.Unlock()
		return
	}
	var data struct {
		Active   int     `json:"active"`
		Streams  int     `json:"streams"`
		Waiting  int     `json:"waiting"`
		Inflight int     `json:"inflight"`
		Retries  int     `json:"retries"`
		Errors   int     `json:"errors"`
		TPS      float64 `json:"tps"`
		RPS      float64 `json:"rps"`
		Sessions []struct {
			Model    string `json:"model"`
			Combo    string `json:"combo"`
			Provider string `json:"provider"`
			Step     string `json:"step"`
			Status   string `json:"status"`
			Finished bool   `json:"finished"`
			TtfbMs   int64  `json:"ttfbMs"`
			Elapsed  int64  `json:"elapsedMs"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}
	live := make([]widgetSession, 0, 4)
	maxSess := 4
	for _, s := range data.Sessions {
		if s.Finished || len(live) >= maxSess {
			continue
		}
		name := s.Combo
		if name == "" {
			name = s.Model
		}
		if len(name) > 20 {
			name = name[:19] + "…"
		}
		live = append(live, widgetSession{
			name:     name,
			step:     s.Step,
			status:   s.Status,
			ageSec:   int(s.Elapsed / 1000),
			provider: s.Provider,
			ttfbMs:   s.TtfbMs,
		})
	}
	sort.Slice(live, func(i, j int) bool { return live[i].ageSec > live[j].ageSec })
	wstate.mu.Lock()
	wstate.online = true
	wstate.authed = true
	wstate.active = data.Active
	wstate.streams = data.Streams
	wstate.waiting = data.Waiting
	wstate.inflight = data.Inflight
	wstate.retries = data.Retries
	wstate.errors = data.Errors
	wstate.tps = data.TPS
	wstate.rps = data.RPS
	wstate.sessions = live
	wstate.note = ""
	wstate.mu.Unlock()
}

func widgetVisible(hwnd windows.HWND) bool {
	ret, _, _ := pIsWindowVisible.Call(uintptr(hwnd))
	return ret != 0
}

func widgetSetVisible(hwnd windows.HWND, show bool) {
	if show {
		_, _, _ = pShowWindow.Call(uintptr(hwnd), swShow)
		_, _, _ = pSetWindowPos.Call(uintptr(hwnd), ^uintptr(0), 0, 0, 0, 0, swpNoMove|swpNoSize)
		_, _, _ = pInvalidateRect.Call(uintptr(hwnd), 0, 0)
	} else {
		_, _, _ = pShowWindow.Call(uintptr(hwnd), swHide)
	}
}

// widgetApplyRound clips the window to a rounded rectangle so the pill
// looks like a proper widget. Re-applied on every resize.
func widgetApplyRound(hwnd windows.HWND, w, h, r int) {
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(r), uintptr(r))
	if rgn != 0 {
		_, _, _ = pSetWindowRgn.Call(uintptr(hwnd), rgn, 1)
	}
}

// widgetMenu shows the right-click menu: show/hide, close.
func widgetMenu(hwnd windows.HWND, x, y int32) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	// Pin UTF-16 label buffers until TrackPopupMenu returns; otherwise the
	// GC may free them mid-call and the menu paints blank white after idle.
	var pinned []*uint16
	addItem := func(id int, label string) {
		p, _ := windows.UTF16PtrFromString(label)
		pinned = append(pinned, p)
		_, _, _ = pAppendMenuW.Call(menu, mfString, uintptr(id), uintptr(unsafe.Pointer(p)))
	}
	if widgetVisible(hwnd) {
		addItem(cmdShow, "Sembunyikan")
	} else {
		addItem(cmdShow, "Tampilkan")
	}
	addItem(cmdClose, "Tutup widget")
	_, _, _ = pSetForegroundWindow.Call(uintptr(hwnd))
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(x), uintptr(y), 0, uintptr(hwnd), 0)
	runtime.KeepAlive(pinned)
	widgetRunCommand(hwnd, int(cmd))
}

func widgetRunCommand(hwnd windows.HWND, cmd int) {
	switch cmd {
	case cmdClose:
		_, _, _ = pPostMessageW.Call(uintptr(hwnd), wmDestroy, 0, 0)
	case cmdShow:
		widgetSetVisible(hwnd, !widgetVisible(hwnd))
	}
}

func widgetTrayTip() string {
	wstate.mu.Lock()
	defer wstate.mu.Unlock()
	if !wstate.online {
		return "9router: offline"
	}
	if wstate.active > 0 {
		return fmt.Sprintf("9router: %d berjalan", wstate.active)
	}
	return "9router: idle"
}

func widgetTray(hwnd windows.HWND, op uintptr, tip string) bool {
	var nid notifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = trayMsg

	// Try to load custom icon from file first. .ico goes through LoadImageW
	// (proper multi-size icon); JPEG/PNG go through GDI+.
	if trayIconPath != "" {
		var icon windows.Handle
		if strings.HasSuffix(strings.ToLower(trayIconPath), ".ico") {
			if trayIconIco == 0 {
				p, _ := windows.UTF16PtrFromString(trayIconPath)
				h, _, _ := pLoadImageW.Call(
					0, uintptr(unsafe.Pointer(p)), 1, 16, 16, 0x10|0x20,
				)
				trayIconIco = windows.Handle(h)
			}
			icon = trayIconIco
		} else {
			if trayIconCache == 0 {
				trayIconCache = loadIconFromImage(trayIconPath)
			}
			icon = trayIconCache
		}
		if icon != 0 {
			nid.hIcon = icon
			utf, _ := windows.UTF16FromString(tip)
			copy(nid.szTip[:], utf)
			ret, _, _ := pShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(&nid)))
			return ret != 0
		}
	}

	// Fallback to default icon
	icon, _, _ := pLoadIconW.Call(0, idiApp)
	nid.hIcon = windows.Handle(icon)
	utf, _ := windows.UTF16FromString(tip)
	copy(nid.szTip[:], utf)
	ret, _, _ := pShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(&nid)))
	return ret != 0
}

func widgetWndProc(hwndRaw, msgRaw, wParam, lParam uintptr) uintptr {
	hwnd := windows.HWND(hwndRaw)
	msg := uint32(msgRaw)
	if taskbarCreatedMsg != 0 && msg == taskbarCreatedMsg {
		widgetTray(hwnd, nimAdd, widgetTrayTip())
		return 0
	}
	if msg == trayMsg {
		// lParam carries the mouse message over the tray icon.
		switch uint32(lParam) {
		case wmLButtonUp:
			widgetSetVisible(hwnd, !widgetVisible(hwnd))
		case wmRButtonUp:
			var pt wPoint
			_, _, _ = pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			widgetMenu(hwnd, pt.x, pt.y)
		}
		return 0
	}
	switch msg {
	case wmLButtonDown:
		_, _, _ = pSendMessageW.Call(uintptr(hwnd), wmNCLButtonDwn, htCaption, 0)
		return 0
	case wmLButtonDbl:
		// Double-click tidak melakukan apa-apa (hanya compact mode).
		return 0
	case wmRButtonUp:
		var pt wPoint
		_, _, _ = pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		widgetMenu(hwnd, pt.x, pt.y)
		return 0
	case wmTimer:
		if wParam == 2 {
			// Animation tick: advance always so the panda breathes on idle
			// and orbits while running. Cheap repaint at 150ms.
			if widgetVisible(hwnd) {
				widgetPulse.Add(1)
				_, _, _ = pInvalidateRect.Call(uintptr(hwnd), 0, 0)
			}
			return 0
		}
		go func() {
			if !widgetFetchBusy.CompareAndSwap(false, true) {
				return // previous poll still in flight; skip instead of piling up
			}
			defer widgetFetchBusy.Store(false)
			widgetFetch()
			if !widgetTray(hwnd, nimModify, widgetTrayTip()) {
				widgetTray(hwnd, nimAdd, widgetTrayTip())
			}
			_, _, _ = pInvalidateRect.Call(uintptr(hwnd), 0, 0)
		}()
		return 0
	case wmPaint:
		widgetPaint(hwnd)
		return 0
	case wmCommand:
		widgetRunCommand(hwnd, int(wParam))
		return 0
	case wmDestroy:
		widgetLog("stop-destroy")
		_, _, _ = pKillTimer.Call(uintptr(hwnd), 1)
		_, _, _ = pKillTimer.Call(uintptr(hwnd), 2)
		widgetTray(hwnd, nimDelete, "")
		_ = os.Remove(widgetPidPath())
		_, _, _ = pPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func widgetText(hdc windows.Handle, font uintptr, x, y int32, s string, color uint32) {
	_, _, _ = pSelectObject.Call(uintptr(hdc), font)
	_, _, _ = pSetTextColor.Call(uintptr(hdc), uintptr(color))
	utf, _ := windows.UTF16FromString(s)
	if len(utf) == 0 {
		return
	}
	_, _, _ = pTextOutW.Call(uintptr(hdc), uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&utf[0])), uintptr(len(utf)-1))
}

func widgetRunning() bool {
	wstate.mu.Lock()
	defer wstate.mu.Unlock()
	return wstate.online && wstate.active > 0
}

// fmtTokMin renders tokens/sec as "6.2k tok/min" like the reference widget.
func fmtTokMin(tps float64) string {
	v := tps * 60
	if v >= 1000 {
		return fmt.Sprintf("%.1fk tok/min", v/1000)
	}
	return fmt.Sprintf("%.0f tok/min", v)
}

func fmtAge(sec int) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	m := sec / 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, sec%60)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

// widgetBar draws one meter row: label + pct on top, 14 segmented bars
// below (claude-code-meter style). Returns the y for the next row.
func widgetBar(hdc windows.Handle, font uintptr, x, y, w int32, label string, frac float64, fill, track uintptr, pctColor uint32) int32 {
	const segs = int32(14)
	gap := int32(2)
	segW := (w - gap*(segs-1)) / segs
	lit := int32(frac*float64(segs) + 0.5)
	if lit < 1 && frac > 0 {
		lit = 1
	}
	if lit > segs {
		lit = segs
	}
	ph := widgetPulse.Load() % 6
	by := y + 14
	bh := int32(10)
	for i := int32(0); i < segs; i++ {
		sx := x + i*(segW+gap)
		var sr [4]int32
		top := by
		if widgetRunning() && i < lit && i == lit-1 {
			top -= ph % 3
		}
		sr[0], sr[1], sr[2], sr[3] = sx, top, sx+segW, by+bh
		if i < lit {
			_, _, _ = pFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&sr)), fill)
		} else {
			_, _, _ = pFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&sr)), track)
		}
	}
	// Label left, percentage right — meter row on top of the bars.
	widgetText(hdc, font, x, y, label, uint32(0x009CA3AF))
	pctText := fmt.Sprintf("%d%%", int(frac*100))
	widgetText(hdc, font, x+w-int32(len(pctText)*7)-4, y, pctText, pctColor)
	return y + 30
}

// 9router brand palette (BGR COLORREF): hitam background, hijau accent.
// Dipakai widgetPaint & paintClaudeIcon. Didefinisikan paket-level biar
// bisa dipakai lintas-fungsi.
const (
	colorBg       = 0x000A0A0A // #0a0a0a brand hitam
	colorGreen    = 0x0055C522 // #22c55e brand hijau
	colorGreenDim = 0x00A3E236 // hijau redup untuk teks/metis
	colorText     = 0x00FAFAFA
	colorMuted    = 0x00A1A1A2
	colorTrack    = 0x001C1C1E // surface hitam
)

func widgetPaint(hwnd windows.HWND) {
	var ps wPaintStruct
	hdcRaw, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	hdc := windows.Handle(hdcRaw)
	defer pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))

	w, h := smallW, smallH

	// Double buffer: draw to a memory DC, blit once — no flicker on hover,
	// click, or timer repaints.
	memRaw, _, _ := pCreateCompatibleDC.Call(uintptr(hdc))
	mem := windows.Handle(memRaw)
	if mem == 0 {
		return
	}
	defer pDeleteDC.Call(memRaw)
	bmpRaw, _, _ := pCreateCompatibleBitmap.Call(uintptr(hdc), uintptr(w), uintptr(h))
	if bmpRaw == 0 {
		return
	}
	defer pDeleteObject.Call(bmpRaw)
	_, _, _ = pSelectObject.Call(memRaw, bmpRaw)

	brush, _, _ := pCreateSolidBrush.Call(uintptr(colorBg))
	accentBrush, _, _ := pCreateSolidBrush.Call(uintptr(wcfg.accent))
	grayBrush, _, _ := pCreateSolidBrush.Call(uintptr(0x006B7280))
	redBrush, _, _ := pCreateSolidBrush.Call(uintptr(0x007171F8))
	amberBrush, _, _ := pCreateSolidBrush.Call(uintptr(0x0024BFFB))
	trackBrush, _, _ := pCreateSolidBrush.Call(uintptr(0x00262B33))
	defer pDeleteObject.Call(brush)
	defer pDeleteObject.Call(accentBrush)
	defer pDeleteObject.Call(grayBrush)
	defer pDeleteObject.Call(redBrush)
	defer pDeleteObject.Call(amberBrush)
	defer pDeleteObject.Call(trackBrush)

	font, _, _ := pCreateFontW.Call(
		uintptr(16), 0, 0, 0, uintptr(500), 0, 0, 0,
		uintptr(1), 0, 0, uintptr(5), 0, uintptr(unsafe.Pointer(widgetFont)),
	)
	defer pDeleteObject.Call(font)
	_, _, _ = pSetBkMode.Call(memRaw, transparentBk)
	oldFont, _, _ := pSelectObject.Call(memRaw, font)

	var rc [4]int32
	rc[0], rc[1], rc[2], rc[3] = 0, 0, int32(w), int32(h)
	_, _, _ = pFillRect.Call(memRaw, uintptr(unsafe.Pointer(&rc)), brush)

	wstate.mu.Lock()
		online := wstate.online
		active := wstate.active
		streams, retries, waiting := wstate.streams, wstate.retries, wstate.waiting
		rps := wstate.rps
		note := wstate.note
		wstate.mu.Unlock()

		// Header: Claude icon + name + LIVE state
		running := online && active > 0

		paintCompact(mem, font, running, note,
			active, streams, retries, waiting, rps)
		_, _, _ = pBitBlt.Call(uintptr(hdc), 0, 0, uintptr(w), uintptr(h), memRaw, 0, 0, srccopy)
		_, _, _ = pSelectObject.Call(memRaw, oldFont)
			return

}

// paintDots draws the cute typing-indicator wave (three hopping dots) used
// while a request is running. Phase comes from the animation timer.
func paintDots(mem windows.Handle, brush uintptr, x, y int32) {
	ph := widgetPulse.Load() % 6
	prev, _, _ := pSelectObject.Call(uintptr(mem), brush)
	for i := int32(0); i < 3; i++ {
		o := (ph + i*2) % 6
		var dy int32
		if o < 3 {
			dy = -o * 2
		} else {
			dy = (o - 6) * 2
		}
		cx := x + i*12
		_, _, _ = pEllipse.Call(uintptr(mem), uintptr(cx), uintptr(y+dy), uintptr(cx+7), uintptr(y+7+dy))
	}
	_, _, _ = pSelectObject.Call(uintptr(mem), prev)
}

// paintClaudeIcon draws a green 9router-style rounded square with an
// abstract shape inside. Idle: diam (no movement). Running: meloncat vertikal
// (bounce) sesuai permintaan. Phase datang dari widgetPulse.
func paintClaudeIcon(mem windows.Handle, x, y, size int32, running bool) {
	ph := widgetPulse.Load()
	// Idle: diam. Running: bounce vertikal.
	scale := int32(100)
	bounce := int32(0)
	if running {
		b := int32(ph % 6)
		if b <= 3 {
			bounce = -b * 2 // naik sampai -6px
		} else {
			bounce = -(6 - b) * 2 // turun balik
		}
	}
	s := int32(int64(size) * int64(scale) / 100)
	// Center the icon
	ox := x + (size-s)/2
	oy := y + (size-s)/2 + bounce

	// Green background square (rounded)
	greenBrush, _, _ := pCreateSolidBrush.Call(uintptr(colorGreen))
	defer pDeleteObject.Call(greenBrush)
	prevBrush, _, _ := pSelectObject.Call(memRawIfAny(mem), greenBrush)
	var ir [4]int32
	ir[0], ir[1], ir[2], ir[3] = ox, oy, ox+s, oy+s
	// Use Ellipse as a rounded rect approximation — simpler and faster in GDI
	pEllipse.Call(memRawIfAny(mem), uintptr(ox), uintptr(oy), uintptr(ox+s), uintptr(oy+s))

	// Dark abstract "R"-ish shape inside (tiga arc membentuk R)
	whiteBrush, _, _ := pCreateSolidBrush.Call(uintptr(0x000A0A0A))
	defer pDeleteObject.Call(whiteBrush)
	_, _, _ = pSelectObject.Call(memRawIfAny(mem), whiteBrush)
	inset := s / 3
	// Top arc
	_, _, _ = pEllipse.Call(memRawIfAny(mem),
		uintptr(ox+inset), uintptr(oy+inset),
		uintptr(ox+s-inset), uintptr(oy+inset+inset))
	// Bottom arc
	_, _, _ = pEllipse.Call(memRawIfAny(mem),
		uintptr(ox+inset), uintptr(oy+s-inset-inset),
		uintptr(ox+s-inset), uintptr(oy+s-inset))
	// Left vertical bar
	_, _, _ = pEllipse.Call(memRawIfAny(mem),
		uintptr(ox+inset), uintptr(oy+inset+inset/2),
		uintptr(ox+inset+inset/2), uintptr(oy+s-inset-inset/2))
	_, _, _ = pSelectObject.Call(memRawIfAny(mem), prevBrush)
}

// memRawIfAny is a no-op helper — mem is already a windows.Handle.
func memRawIfAny(mem windows.Handle) uintptr { return uintptr(mem) }

var widgetSpinner = []string{"|", "/", "—", "\\"}

// paintCompact draws the slim elongated pill: one header row with the LIVE
// state, one dim stats row with a spinner. Uses an orange Claude Code icon
// that breathes when idle and wiggles when running.
func paintCompact(mem windows.Handle, font uintptr, running bool, note string, active, streams, retries, waiting int, rps float64) {
	// Panda icon: bob 1px on idle breathe so idle tetap terlihat hidup;
	// orbit penuh hanya saat running.
	ph := widgetPulse.Load()
	idleBob := int32(0)
	if !running {
		if ph%8 < 4 {
			idleBob = 1
		}
	}
	if !drawWidgetIcon(mem, 12, 14+idleBob, 20) {
		paintClaudeIcon(mem, 12, 14+idleBob, 20, running)
	}
	if running {
		paintOrbit(mem, 22, 24, int(widgetPulse.Load()))
	}
	status := "idle"
	statusColor := uint32(colorMuted)
	if running {
		status = "live"
		statusColor = colorGreen
	}
	line := note
	if line == "" {
		line = fmt.Sprintf("%d aktif · %.1f req/s", active, rps)
	}
	widgetText(mem, font, 40, 16, line, colorText)
	widgetText(mem, font, smallW-12-int32(len(status)*7), 16, status, statusColor)
}

func mustUTF16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func widgetCrashLog() string {
	if tmp := os.Getenv("TEMP"); tmp != "" {
		return filepath.Join(tmp, "9router-widget.log")
	}
	return "9router-widget.log"
}

// widgetLog appends one lifecycle line (start/stop). Temporary forensics for
// the auto-spawn death; cheap enough to keep.
func widgetLog(ev string) {
	line := fmt.Sprintf("%s pid=%d %s\n", time.Now().Format("15:04:05"), os.Getpid(), ev)
	f, err := os.OpenFile(widgetCrashLog(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}

// widgetPidPath tracks the widget process so `status`/`stop` can tell it
// apart from the server (both are 9router-go.exe).
func widgetPidPath() string {
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "9router-go-widget.pid")
}

func debugStack() string { return string(debug.Stack()) }

// widgetSingletonActive reports whether a widget instance is alive (or
// still booting), via the same named mutex runWidget holds. The test handle
// is closed immediately so it never blocks a real widget from starting.
func widgetSingletonActive() bool {
	name, _ := windows.UTF16PtrFromString("9routerWidgetSingleton")
	h, _, _ := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return false
	}
	errCode, _, _ := pGetLastError.Call()
	_, _, _ = pCloseHandleW.Call(h)
	return errCode == errAlreadyExists
}

func widgetFlag(args []string, name, def string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--"+name+"=") {
			return strings.TrimPrefix(a, "--"+name+"=")
		}
		if a == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return def
}

// runWidget opens the native widget window. Called from main on Windows.
func runWidget(args []string) {
	runtime.LockOSThread()
	// Render at native pixels instead of letting Windows bitmap-stretch the
	// window on scaled displays (125%+) — that stretch is what made the
	// text and logo look blurry.
	_, _, _ = pSetProcessDPIAware.Call()
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("%s widget panic: %v\n%s\n", time.Now().Format(time.RFC3339), r, debugStack())
			_ = os.WriteFile(widgetCrashLog(), []byte(msg), 0644)
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(1)
		}
	}()

	// Single instance: a second launch (e.g. auto-start firing while the
	// widget already runs) exits quietly instead of stacking windows.
	name, _ := windows.UTF16PtrFromString("9routerWidgetSingleton")
	mutex, _, _ := pCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(name)))
	if mutex == 0 {
		fmt.Fprintln(os.Stderr, "widget: mutex gagal")
		os.Exit(1)
	}
	if errCode, _, _ := pGetLastError.Call(); errCode == errAlreadyExists {
		fmt.Println("widget sudah berjalan")
		os.Exit(0)
	}
	_ = os.WriteFile(widgetPidPath(), []byte(strconv.Itoa(os.Getpid())), 0644)
	widgetLog("start")

	wcfg = widgetCfg{
		host:     widgetFlag(args, "host", "127.0.0.1"),
		port:     widgetFlag(args, "port", envOr("PORT", "20130")),
		bg:       parseHexColor(widgetFlag(args, "bg", "131114"), 0x00141113),
		accent:   parseHexColor(widgetFlag(args, "accent", "22c55e"), 0x0055C522),
		password: widgetFlag(args, "password", ""),
	}
	// Load custom tray icon if specified, or use default logo path
	if imgPath := widgetFlag(args, "icon", ""); imgPath != "" {
		trayIconPath = imgPath
	} else if trayIconPath == "" {
		// Default to the 9router-go icon (icon-192.png) next to the exe
		if exe, err := os.Executable(); err == nil {
			trayIconPath = filepath.Join(filepath.Dir(exe), "web", "public", "icons", "icon-192.png")
		} else {
			trayIconPath = filepath.Join(os.Getenv("USERPROFILE"), "Downloads", "panda.jpg")
		}
	}
	op, _ := strconv.Atoi(widgetFlag(args, "opacity", "235"))
	if op < 40 {
		op = 40
	}
	if op > 255 {
		op = 255
	}
	wcfg.opacity = byte(op)

	jar, _ := cookiejar.New(nil)
	wclient = &http.Client{Jar: jar, Timeout: 5 * time.Second}
	if wcfg.password != "" {
		widgetLogin()
	}
	widgetFetch()

	className := widgetClsName
	hInstRaw, _, _ := pGetModuleHandleW.Call(0)
	if hInstRaw == 0 {
		fmt.Fprintln(os.Stderr, "widget: GetModuleHandle gagal")
		os.Exit(1)
	}
	// Resolve the icon BEFORE RegisterClassEx: the class copies this struct,
	// so assigning hIcon afterwards (old code) left the class with a null
	// icon and the pill lost its icon after idle/GC pressure.
	var clsIcon windows.Handle
	if trayIconPath != "" {
		if strings.HasSuffix(strings.ToLower(trayIconPath), ".ico") {
			p, _ := windows.UTF16PtrFromString(trayIconPath)
			h, _, _ := pLoadImageW.Call(0, uintptr(unsafe.Pointer(p)), 1, 32, 32, 0x10|0x20)
			clsIcon = windows.Handle(h)
		} else {
			clsIcon = loadIconFromImage(trayIconPath)
		}
		if clsIcon == 0 {
			icon, _, _ := pLoadIconW.Call(0, idiApp)
			clsIcon = windows.Handle(icon)
		}
		widgetHIcon = clsIcon
	}
	wc := wndClassEx{
		style:         csDblClks,
		lpfnWndProc:   wndProcAddr,
		hInstance:     windows.Handle(hInstRaw),
		hIcon:         clsIcon,
		hIconSm:       clsIcon,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if ret, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		fmt.Fprintln(os.Stderr, "widget: RegisterClassEx gagal")
		os.Exit(1)
	}

	// Window icon already resolved into wc.hIcon / widgetHIcon above.

	title := widgetTitle
	const W, H = smallW, smallH
	screenW, _, _ := pGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := pGetSystemMetrics.Call(1) // SM_CYSCREEN
	x := int32(screenW) - W - 16
	if x < 0 {
		x = 0
	}
	y := int32(screenH) - H - 12 // margin 12px di atas taskbar
	if y < 0 {
		y = 0
	}
	widgetPos.mu.Lock()
	widgetPos.x, widgetPos.y = x, y
	widgetPos.mu.Unlock()
	if taskbarCreatedMsg == 0 {
		p, _ := windows.UTF16PtrFromString("TaskbarCreated")
		ret, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(p)))
		taskbarCreatedMsg = uint32(ret)
	}
	hwndRaw, _, _ := pCreateWindowExW.Call(
		wsExTopmost|wsExLayered|wsExToolwindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup|wsVisible,
		uintptr(x), uintptr(y), uintptr(W), uintptr(H),
		0, 0, uintptr(wc.hInstance), 0,
	)
	if hwndRaw == 0 {
		fmt.Fprintln(os.Stderr, "widget: CreateWindow gagal")
		os.Exit(1)
	}
	hwnd := windows.HWND(hwndRaw)
	_, _, _ = pSetLayeredWindowAttr.Call(hwndRaw, 0, uintptr(wcfg.opacity), lwaAlpha)
	widgetApplyRound(hwnd, smallW, smallH, smallH/2)
	_, _, _ = pSetTimer.Call(hwndRaw, 1, 1000, 0)
	_, _, _ = pSetTimer.Call(hwndRaw, 2, 150, 0)
	_, _, _ = pShowWindow.Call(hwndRaw, swShowNoActive)
	_, _, _ = pUpdateWindow.Call(hwndRaw)
	// Tray icon: the way back when the window is hidden, plus a live tooltip.
	widgetTray(hwnd, nimAdd, widgetTrayTip())

	var msg wMsg
	for {
		ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			break // WM_QUIT
		}
		_, _, _ = pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	_ = hwnd
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
