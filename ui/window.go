package ui

// ---------------------------------------------------------------------------
// Window management: every window has a draggable title bar, click-to-raise
// z-order, optional [×] close (reopen = its toggle key), screen-clamped
// dragging and per-session position persistence.
// ---------------------------------------------------------------------------

// WinID identifies a managed UI window.
type WinID string

// The managed windows.
const (
	WinStation   WinID = "station"
	WinOverview  WinID = "overview"
	WinSelected  WinID = "selected"
	WinLog       WinID = "log"
	WinModules   WinID = "modules"
	WinScanner   WinID = "scanner"
	WinInventory WinID = "inventory"
	WinFitting   WinID = "fitting"
	WinStarmap   WinID = "starmap"
	WinHelp      WinID = "help"
)

// defaultZOrder is the back-to-front draw/hit order at session start. The
// station sits at the back so the combat log stays visible over it while
// docked (log clicks fall through to the station); raising a window moves
// it to the front. Starmap/Help are drawn by their scenes but keep their
// position/z state here.
var defaultZOrder = []WinID{
	WinStation, WinOverview, WinSelected, WinLog, WinModules,
	WinScanner, WinInventory, WinFitting, WinStarmap, WinHelp,
}

// closableWindows are the windows with a [×] button; their toggle key
// reopens them (I inventory, F fitting, C scanner, N starmap, H help).
// Keyless HUD panels (overview/selected/log/modules/station) are draggable
// but cannot be closed — nothing would bring them back.
var closableWindows = map[WinID]bool{
	WinScanner: true, WinInventory: true, WinFitting: true,
	WinStarmap: true, WinHelp: true,
}

// windowTitles are the default title-bar labels.
var windowTitles = map[WinID]string{
	WinOverview:  "Overview",
	WinSelected:  "Selected Item",
	WinLog:       "Log",
	WinModules:   "Modules",
	WinScanner:   "Scanner",
	WinInventory: "Inventory",
	WinFitting:   "Fitting",
	WinStation:   "Station Services",
	WinStarmap:   "Starmap",
	WinHelp:      "Help — Entieve key map",
}

// WindowTitle returns the title-bar label of a window.
func WindowTitle(id WinID) string {
	if t, ok := windowTitles[id]; ok {
		return t
	}
	return string(id)
}

// winState is one window's per-session position/open state.
type winState struct {
	rect  Rect
	moved bool
	open  bool
}

// ---------------------------------------------------------------------------
// Pure window math (tested)
// ---------------------------------------------------------------------------

// ClampRectToScreen keeps r fully inside a w×h screen (rects larger than
// the screen shrink to it, anchored top-left).
func ClampRectToScreen(r Rect, w, h float64) Rect {
	if r.W > w {
		r.W = w
	}
	if r.H > h {
		r.H = h
	}
	r.X = clamp(r.X, 0, w-r.W)
	r.Y = clamp(r.Y, 0, h-r.H)
	return r
}

// TitleGrabOffset returns the grab offset for a drag started at (x, y) on
// r's title bar. The point must lie in the strip and outside every
// accessory rect (close/filter buttons); otherwise ok is false.
func TitleGrabOffset(r Rect, x, y float64, exclude ...Rect) (dx, dy float64, ok bool) {
	if !TitleBarRect(r).Contains(x, y) {
		return 0, 0, false
	}
	for _, e := range exclude {
		if e.Contains(x, y) {
			return 0, 0, false
		}
	}
	return x - r.X, y - r.Y, true
}

// DragRect moves r so the point that started the drag follows (mx, my),
// with the result clamped to the w×h screen.
func DragRect(r Rect, grabDX, grabDY, mx, my, w, h float64) Rect {
	return ClampRectToScreen(Rect{X: mx - grabDX, Y: my - grabDY, W: r.W, H: r.H}, w, h)
}

// Raise returns the z-order with id moved to the front (last). Unknown ids
// and already-front ids leave the order unchanged.
func Raise(z []WinID, id WinID) []WinID {
	idx := -1
	for i, v := range z {
		if v == id {
			idx = i
		}
	}
	if idx < 0 || idx == len(z)-1 {
		return z
	}
	out := make([]WinID, 0, len(z))
	out = append(out, z[:idx]...)
	out = append(out, z[idx+1:]...)
	return append(out, id)
}

// DefaultWindowRect maps a window id to its default rect at this screen
// size (the non-overlapping layout the tests pin).
func DefaultWindowRect(id WinID, reg Regions) Rect {
	switch id {
	case WinStation:
		return reg.Station
	case WinOverview:
		return reg.Overview
	case WinSelected:
		return reg.Selected
	case WinLog:
		return reg.Log
	case WinModules:
		return reg.Modules
	case WinScanner:
		return reg.Scanner
	case WinInventory:
		return reg.Inventory
	case WinFitting:
		return reg.Fitting
	case WinStarmap:
		return reg.Starmap
	case WinHelp:
		return reg.Help
	}
	return Rect{}
}

// ---------------------------------------------------------------------------
// UI-side window state
// ---------------------------------------------------------------------------

// initWindows seeds window state with the default layout.
func (u *UI) initWindows() {
	u.wins = make(map[WinID]*winState, len(defaultZOrder))
	u.zorder = append([]WinID(nil), defaultZOrder...)
	for _, id := range defaultZOrder {
		u.wins[id] = &winState{}
	}
	// Overlays start closed; the HUD windows are always on.
	for _, id := range []WinID{WinStation, WinOverview, WinSelected, WinLog, WinModules} {
		u.wins[id].open = true
	}
}

// winStateOf returns (allocating lazily) the state of a window.
func (u *UI) winStateOf(id WinID) *winState {
	st, ok := u.wins[id]
	if !ok {
		st = &winState{}
		u.wins[id] = st
	}
	return st
}

// WindowOpen reports whether a window is currently open.
func (u *UI) WindowOpen(id WinID) bool { return u.winStateOf(id).open }

// SetWindowOpen opens/closes a window; opening raises it to the front.
func (u *UI) SetWindowOpen(id WinID, open bool) {
	st := u.winStateOf(id)
	was := st.open
	st.open = open
	if open && !was {
		u.raise(id)
		if id == WinScanner {
			// A fresh window starts a fresh sweep with current contacts.
			u.scanT = 0
			u.scanCounts = nil
		}
	}
}

// ToggleWindow flips a window's visibility (the toggle key path).
func (u *UI) ToggleWindow(id WinID) { u.SetWindowOpen(id, !u.WindowOpen(id)) }

// WindowRect returns the current rect of a window: the moved (persisted,
// screen-clamped) position, or its default layout rect.
func (u *UI) WindowRect(id WinID, reg Regions) Rect {
	st := u.winStateOf(id)
	if !st.moved {
		return DefaultWindowRect(id, reg)
	}
	return ClampRectToScreen(st.rect, u.screenW, u.screenH)
}

// raise moves a window to the front of the z-order. The station overlay is
// exempt: it lives at the back so the combat log (drawn over it while
// docked) never gets buried by a station click, and its clicks fall
// through the contentless log to it anyway.
func (u *UI) raise(id WinID) {
	if id == WinStation {
		return
	}
	u.zorder = Raise(u.zorder, id)
}

// visibleWindows returns the open windows that are currently on screen
// (docked hides the space HUD), in draw order (back to front).
func (u *UI) visibleWindows() []WinID {
	docked := u.docked()
	out := make([]WinID, 0, len(u.zorder))
	for _, id := range u.zorder {
		st := u.winStateOf(id)
		if !st.open {
			continue
		}
		switch id {
		case WinStation:
			if !docked {
				continue
			}
		case WinOverview, WinSelected, WinModules, WinScanner:
			if docked {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

// clickFallsThrough reports whether a click on this window's body should
// pass to the window beneath (the log has no content and sits over the
// station panel while docked).
func clickFallsThrough(id WinID) bool { return id == WinLog }

// chromeAction is what a press did to a window's chrome.
type chromeAction int

const (
	chromePass   chromeAction = iota // not on this window's chrome
	chromeGrab                       // title bar: raise + drag
	chromeClose                      // [×] button
	chromeFilter                     // overview filter toggle
)

// chromeAt hit-tests the chrome buttons/title bar of window id at rect r.
func (u *UI) chromeAt(id WinID, r Rect, x, y float64) chromeAction {
	if closableWindows[id] && CloseButtonRect(r).Contains(x, y) {
		return chromeClose
	}
	if id == WinOverview && OverviewFilterButtonRect(r).Contains(x, y) {
		return chromeFilter
	}
	if _, _, ok := TitleGrabOffset(r, x, y); ok {
		return chromeGrab
	}
	return chromePass
}

// OverlayInput handles window chrome (title-bar drag, [×] close) for
// scene-drawn windows (starmap, help). It reports whether the input was
// consumed by chrome, so the caller can skip content handling. Drag
// continuation consumes every frame the button stays held.
func (u *UI) OverlayInput(in Input, id WinID) bool {
	if u.drag != nil {
		if u.drag.id != id {
			return false
		}
		u.dragUpdate(in)
		return true
	}
	if !in.JustClicked[0] {
		return false
	}
	r := u.WindowRect(id, u.reg())
	if !r.Contains(in.MouseX, in.MouseY) {
		return false
	}
	switch u.chromeAt(id, r, in.MouseX, in.MouseY) {
	case chromeClose:
		u.SetWindowOpen(id, false)
		return true
	case chromeGrab:
		dx, dy, _ := TitleGrabOffset(r, in.MouseX, in.MouseY)
		u.raise(id)
		u.drag = &winDrag{id: id, dx: dx, dy: dy}
		return true
	}
	return false
}
