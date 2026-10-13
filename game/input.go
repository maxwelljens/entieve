package game

import (
	"github.com/entieve/entieve/uikit"
	"github.com/hajimehoshi/ebiten/v2"
)

// Binding documents one keyboard binding. KeyMap is the single source of
// truth for the in-game help overlay (H) and docs/GAMEPLAY.md.
//
// Conflict resolutions (documented): F1–F8 are high-slot module toggles, so
// the classic "F5 save" moved to Ctrl+S (F9 load kept: no conflict). "M" is
// mine; station services/market is "G".
type Binding struct {
	Key    ebiten.Key
	Shift  bool
	Ctrl   bool
	Action string // stable action id
	Desc   string // help-overlay text
}

// KeyMap lists every keyboard binding. No (Key, Shift, Ctrl) combination
// appears twice (tested); several keys may share an action.
var KeyMap = []Binding{
	{Key: ebiten.KeyTab, Action: "lock-nearest", Desc: "Tab — cycle-lock nearest target"},
	{Key: ebiten.KeyTab, Shift: true, Action: "lock-selected", Desc: "Shift+Tab — lock selected target"},
	{Key: ebiten.KeyF1, Action: "high-1", Desc: "F1 — toggle high slot 1"},
	{Key: ebiten.KeyF2, Action: "high-2", Desc: "F2 — toggle high slot 2"},
	{Key: ebiten.KeyF3, Action: "high-3", Desc: "F3 — toggle high slot 3"},
	{Key: ebiten.KeyF4, Action: "high-4", Desc: "F4 — toggle high slot 4"},
	{Key: ebiten.KeyF5, Action: "high-5", Desc: "F5 — toggle high slot 5"},
	{Key: ebiten.KeyF6, Action: "high-6", Desc: "F6 — toggle high slot 6"},
	{Key: ebiten.KeyF7, Action: "high-7", Desc: "F7 — toggle high slot 7"},
	{Key: ebiten.KeyF8, Action: "high-8", Desc: "F8 — toggle high slot 8"},
	{Key: ebiten.KeyA, Action: "approach", Desc: "A — approach selected"},
	{Key: ebiten.KeyQ, Action: "orbit", Desc: "Q — orbit selected (at the Selected Item orbit range)"},
	{Key: ebiten.KeyS, Action: "stop", Desc: "S — stop"},
	{Key: ebiten.KeyW, Action: "align", Desc: "W — align to selected"},
	{Key: ebiten.KeyZ, Action: "warp", Desc: "Z — warp to selected (at the Selected Item warp range)"},
	{Key: ebiten.KeyD, Action: "dock", Desc: "D — dock (selected station, else nearest)"},
	{Key: ebiten.KeyJ, Action: "jump", Desc: "J — jump through gate (selected, else nearest)"},
	{Key: ebiten.KeyL, Action: "loot", Desc: "L — loot nearest wreck"},
	{Key: ebiten.KeyL, Shift: true, Action: "log", Desc: "Shift+L — toggle log window"},
	{Key: ebiten.KeyM, Action: "mine", Desc: "M — mine selected asteroid"},
	{Key: ebiten.KeyI, Action: "inventory", Desc: "I — toggle inventory"},
	{Key: ebiten.KeyF, Action: "fitting", Desc: "F — toggle fitting"},
	{Key: ebiten.KeyC, Action: "scanner", Desc: "C — toggle scanner (off-grid sweep)"},
	{Key: ebiten.KeyG, Action: "market", Desc: "G — toggle market window (trade while docked)"},
	{Key: ebiten.KeyN, Action: "starmap", Desc: "N — toggle starmap"},
	{Key: ebiten.KeyH, Action: "help", Desc: "H — toggle help"},
	{Key: ebiten.KeyEscape, Action: "menu", Desc: "Esc — menu (Resume / Save / Load / Quit)"},
	{Key: ebiten.KeyBracketLeft, Action: "time-slower", Desc: "[ — time acceleration down"},
	{Key: ebiten.KeyMinus, Action: "time-slower", Desc: "- — time acceleration down"},
	{Key: ebiten.KeyBracketRight, Action: "time-faster", Desc: "] — time acceleration up"},
	{Key: ebiten.KeyEqual, Action: "time-faster", Desc: "= — time acceleration up"},
	{Key: ebiten.KeyS, Ctrl: true, Action: "save", Desc: "Ctrl+S — save game"},
	{Key: ebiten.KeyF9, Action: "load", Desc: "F9 — load game"},
	{Key: ebiten.KeyArrowUp, Action: "pan-up", Desc: "Up arrow — pan camera up"},
	{Key: ebiten.KeyArrowDown, Action: "pan-down", Desc: "Down arrow — pan camera down"},
	{Key: ebiten.KeyArrowLeft, Action: "pan-left", Desc: "Left arrow — pan camera left"},
	{Key: ebiten.KeyArrowRight, Action: "pan-right", Desc: "Right arrow — pan camera right"},
}

// MouseBinding documents one mouse gesture for the help overlay.
type MouseBinding struct {
	Button string
	Mod    string
	Desc   string
}

// MouseMap documents the mouse controls (same single source as KeyMap).
// Range numbers are derived from the selector arrays, never typed: the
// right-click orbit flies at uikit.OrbitRanges[0] (scene.go), and the
// pin in keymap_test checks the two agree. Right-click on a UI surface
// opens the context menu (uikit/menu.go) instead of acting directly.
var MouseMap = []MouseBinding{
	{Button: "Left click", Desc: "Select entity / press UI buttons; drag a title bar to move a window"},
	{Button: "Left click", Mod: "on column header", Desc: "sort the overview (click again flips direction)"},
	{Button: "Left click", Mod: "Ctrl", Desc: "Lock target under cursor"},
	{Button: "Left double-click", Desc: "Show Info on an overview or table row; approach an entity under the cursor; on empty space, fly straight toward the point"},
	{Button: "Right click", Desc: "Orbit target under cursor (" + uikit.FormatInt(int64(orbitDefaultRange)) + " m)"},
	{Button: "Right click", Mod: "on a UI row or rack cell", Desc: "open the context menu (Show Info, plus the row's actions)"},
	{Button: "Right click", Mod: "on weapon slot", Desc: "open the module menu (Cycle charge walks Auto → cargo ammo)"},
	{Button: "Middle drag", Desc: "Pan camera"},
	{Button: "Wheel", Desc: "Zoom (0.05x – 4x); over the overview, scroll the list"},
}
