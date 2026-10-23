package widgets

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Context menus (EVE-style right-click menus).
//
// A menu is a flat popup owned by the Kit — not an ebitenui ContextMenu
// container (those are per-widget, re-parent and have no disabled state)
// and not a widget.Window either: it is laid out from its entries, drawn
// by Kit.Draw AFTER ui.Draw (so it is always the topmost pixels) and
// hit-tested by Kit.MenuInput, which the host pumps before any scene or
// key handling. While a menu is up the kit skips ui.Update entirely
// (Kit.Update), so no widget underneath can see the click that dismisses
// the popup, and the host gates the world through MenuRect/MenuInput —
// the modality is the menu's own state, never a one-frame-stale hover
// flag.
//
//	┌──────────────────────────┐
//	│ Show Info                1st entry, Bold — every surface menu
//	│ Show Info — Fusion L     secondary refs
//	│ ─────────────────────    MenuSep hairline
//	│ Cycle charge             Disabled entries stay visible, grayed
//	└──────────────────────────┘

// Menu geometry (EVE-flat rows on the tooltip's surface treatment).
const (
	menuPadX    = 10  // popup side padding
	menuPadY    = 5   // popup top/bottom padding
	menuRowPadX = 10  // label inset inside a row band
	menuRowPadY = 4   // vertical padding around a row's line
	menuMinW    = 150 // popup floor width
	menuSepH    = 9   // separator band height
	menuHoverA  = 40  // amber hover fill alpha
	menuTickW   = 14  // the tick gutter of a checked (column) menu
)

// MenuEntry is one context-menu row. The first entry of every surface
// menu is "Show Info", Bold. Disabled entries render grayed but stay
// visible (never hidden); Sep draws a hairline instead of a label. Checked
// is a column menu's on/off state: when a menu carries any checked row,
// every label is inset past a tick gutter and the checked ones draw a tick
// (a runtime polyline — the interface has no font glyph for one).
type MenuEntry struct {
	Label    string
	Bold     bool
	Disabled bool
	Sep      bool
	Checked  bool
	Action   func()
}

// MenuSep is the separator entry: a hairline between menu groups.
var MenuSep = MenuEntry{Sep: true}

// menuRow is one laid-out band: its entry index and screen rect.
type menuRow struct {
	entry int
	rect  image.Rectangle
}

// The consumed-gesture latch — ONE name for the whole same-frame double-action
// guard, whose halves live here and in screens/kit.go:
//
//	set       Menu.Click / Menu.Esc, when the popup consumes a press or Esc;
//	read      Menu.Poll's early return: "this frame already acted", so the
//	          second poll of the same frame (the host's MenuInput pass, then
//	          Kit.Update's fallback) never runs an entry twice — an entry's
//	          Action may have reopened the popup, and running it again would
//	          undo it;
//	cleared   Menu.TakeBlocked, called once per frame by Kit.Update and the
//	          ONLY clearer, so the next frame's poll is live again while
//	          ui.Update stays skipped for the frame the gesture happened in.
//
// It lives on the Menu because the gesture is the popup's own input; Kit.Update
// is its only care-taker. Pinned by the real click in
// widgets.TestColumnMenuToggleAppliesFromTheDotsClick (one toggle, not two)
// and, kit-side, screens.TestRaiseNeverFiresThroughTheMenu.
//
// Menu is one session's context menu: opened through the ActionSink
// (Do(OpenMenu{…})), drawn and gated by the Kit. The zero value is a
// closed menu; NewMenu (through NewEnv/Kit) builds one bound to a Theme.
type Menu struct {
	th theme.Theme

	open    bool
	blocked bool // the consumed-gesture latch: set by Click/Esc, cleared by TakeBlocked

	entries []MenuEntry
	rect    image.Rectangle // popup rect in screen space
	rows    []menuRow       // one band per entry, screen space
	hover   int             // entry index under the pointer, -1 = none
	screen  image.Rectangle // last drawn screen (the clamp bound)
	tick    int             // tick gutter width in px (0 = no checked rows)
}

// NewMenu builds a closed context menu over the given environment.
func NewMenu(d theme.Theme) *Menu { return &Menu{th: d, hover: -1} }

// Open replaces the menu's entries and lays the popup out at at (the
// click position), clamped inside the last screen the kit drew.
func (m *Menu) Open(entries []MenuEntry, at image.Point) {
	m.entries = append([]MenuEntry(nil), entries...)
	m.open = true
	m.hover = -1
	m.layout(at)
}

// Close dismisses the menu. Entries stay readable for inspection; the
// rect empties so the gate can never outlive the popup.
func (m *Menu) Close() {
	m.open = false
	m.rect = image.Rectangle{}
	m.rows = nil
	m.hover = -1
}

// IsOpen reports whether the popup is up.
func (m *Menu) IsOpen() bool { return m.open }

// Rect is the popup's screen rect (zero when closed). The host gates
// world input on it: an ebitenui-drawn popup would be invisible to the
// game's window-rect hit test, this one is not.
func (m *Menu) Rect() image.Rectangle {
	if !m.open {
		return image.Rectangle{}
	}
	return m.rect
}

// Entries returns the menu's entries (nil once never opened).
func (m *Menu) Entries() []MenuEntry { return m.entries }

// RowRect returns entry i's laid-out band in screen space.
func (m *Menu) RowRect(i int) image.Rectangle {
	if i < 0 || i >= len(m.rows) {
		return image.Rectangle{}
	}
	return m.rows[i].rect
}

// HoverIndex is the entry index the pointer sits on (-1 = none).
func (m *Menu) HoverIndex() int { return m.hover }

// Hover moves the highlight to the entry under pt.
func (m *Menu) Hover(pt image.Point) {
	m.hover = -1
	if !m.open {
		return
	}
	if i, ok := m.at(pt); ok {
		m.hover = i
	}
}

// Click handles one pointer press while the menu is up: it always
// consumes the gesture (the menu is modal), closes, and runs the pressed
// entry's Action — after the close, so an action that opens another menu
// wins. Disabled entries and separators only close.
// It sets the consumed-gesture latch (the contract on Menu): an action may
// reopen the popup, and Kit.Update polls again in the same frame after the
// host's MenuInput pass, so a second run must not reach an entry.
func (m *Menu) Click(pt image.Point) bool {
	if !m.open {
		return false
	}
	m.blocked = true
	i, hit := m.at(pt)
	var entry MenuEntry
	if hit {
		entry = m.entries[i]
	}
	m.Close()
	if hit && !entry.Sep && !entry.Disabled && entry.Action != nil {
		entry.Action()
	}
	return true
}

// Esc handles the dismiss key: closes and consumes the frame.
func (m *Menu) Esc() bool {
	if !m.open {
		return false
	}
	m.blocked = true
	m.Close()
	return true
}

// TakeBlocked reads (and clears) the consumed-gesture latch: Kit.Update skips
// ui.Update on the frame it reports true, so the dismiss click never reaches
// the widgets underneath. The only clearer — see the contract on Menu.
func (m *Menu) TakeBlocked() bool {
	b := m.blocked
	m.blocked = false
	return b
}

// at returns the entry index whose band contains pt.
func (m *Menu) at(pt image.Point) (int, bool) {
	for _, r := range m.rows {
		if pt.In(r.rect) {
			return r.entry, true
		}
	}
	return -1, false
}

// layout sizes the popup from its entries and places it at at, slid back
// inside the last known screen. Row bands tile the popup exactly (they
// are the hit test and the hover fill).
func (m *Menu) layout(at image.Point) {
	face := m.th.Face(theme.StepSmall)
	bold := m.th.FaceBold(theme.StepSmall)
	rowPadX, rowPadY := theme.Px(menuRowPadX), theme.Px(menuRowPadY)
	padY := theme.Px(menuPadY)
	rowH := TipLineH(face) + 2*rowPadY
	if b := TipLineH(bold) + 2*rowPadY; b > rowH {
		rowH = b
	}
	width := theme.Px(menuMinW)
	// The tick gutter is per menu, not per row: a column menu's labels all
	// start on one x.
	m.tick = 0
	for _, e := range m.entries {
		if e.Checked {
			m.tick = theme.Px(menuTickW)
			break
		}
	}
	for _, e := range m.entries {
		if e.Sep || e.Label == "" {
			continue
		}
		f := face
		if e.Bold {
			f = bold
		}
		if w := int(TipMeasure(e.Label, f)) + 2*rowPadX + m.tick; w > width {
			width = w
		}
	}
	// Rows tile the popup from the top padding down; the bottom padding
	// follows the last band.
	y := padY
	m.rows = make([]menuRow, 0, len(m.entries))
	for i, e := range m.entries {
		h := rowH
		if e.Sep {
			h = theme.Px(menuSepH)
		}
		m.rows = append(m.rows, menuRow{entry: i, rect: image.Rect(0, y, width, y+h)})
		y += h
	}
	height := y + padY
	// Clamp: a popup opened past an edge slides back inside the screen
	// (no-op mid-screen — the same rule the tooltip placement uses).
	px, py := at.X, at.Y
	if !m.screen.Empty() {
		px = clampAxis(px, width, m.screen.Min.X, m.screen.Max.X)
		py = clampAxis(py, height, m.screen.Min.Y, m.screen.Max.Y)
	}
	m.rect = image.Rect(px, py, px+width, py+height)
	for i := range m.rows {
		m.rows[i].rect = m.rows[i].rect.Add(m.rect.Min)
	}
}

// Draw paints the popup: the tooltip's surface (opaque fill + 1 px
// border), the amber hover band, the labels (Bold first entry, grayed
// when disabled) and the separator hairlines. Kit.Draw calls it last, so
// the menu is always on top.
func (m *Menu) Draw(dst *ebiten.Image) {
	if !m.open {
		return
	}
	m.screen = dst.Bounds()
	p := m.th.Pal()
	theme.DrawRoundBox(dst, m.rect, theme.Solid(p.BGHard), theme.NRGBA(p.Border, 220), 1, theme.AllRadii(theme.RadiusControl))
	for _, r := range m.rows {
		e := m.entries[r.entry]
		if e.Sep {
			y := (r.rect.Min.Y + r.rect.Max.Y) / 2
			theme.FillRect(dst, image.Rect(r.rect.Min.X+theme.Px(menuPadX), y, r.rect.Max.X-theme.Px(menuPadX), y+theme.Px(1)),
				theme.NRGBA(p.Border, 160))
			continue
		}
		if m.hover == r.entry && !e.Disabled {
			theme.FillRect(dst, r.rect, theme.NRGBA(p.Hover, menuHoverA))
		}
		face, col := m.th.Face(theme.StepSmall), theme.Solid(p.Text)
		if e.Bold {
			face = m.th.FaceBold(theme.StepSmall)
		}
		if e.Disabled {
			col = theme.NRGBA(p.Dim, 150)
		}
		if m.tick > 0 {
			m.drawTick(dst, r.rect, e, p)
		}
		theme.DrawCardText(dst, e.Label, face, r.rect.Min.X+theme.Px(menuRowPadX)+m.tick, r.rect.Min.Y+theme.Px(menuRowPadY), col)
	}
}

// drawTick paints a checked entry's tick in its row's gutter: two strokes
// (the menu's dot language, no font glyph), the accent at rest and the
// disabled gray for a row that only reports.
func (m *Menu) drawTick(dst *ebiten.Image, r image.Rectangle, e MenuEntry, p theme.Palette) {
	if !e.Checked {
		return
	}
	col := theme.NRGBA(p.Accent, 255)
	if e.Disabled {
		col = theme.NRGBA(p.Dim, 150)
	}
	s := float32(theme.Scale())
	x := float32(r.Min.X + theme.Px(menuRowPadX))
	y := float32((r.Min.Y + r.Max.Y) / 2)
	vector.StrokeLine(dst, x, y, x+4*s, y+4*s, s, col, true)
	vector.StrokeLine(dst, x+4*s, y+4*s, x+10*s, y-5*s, s, col, true)
}

// pressEdge reports a mouse press edge for menu dismissal/activation.
// Production polls ebiten directly; headless tests cannot generate OS
// mouse events, so the edge read goes through this indirection.
// SetPressEdgeForTest swaps it — tests only; the default is the real poll.
var pressEdge = func(b ebiten.MouseButton) bool {
	return inpututil.IsMouseButtonJustPressed(b)
}

// SetPressEdgeForTest overrides the raw press-edge poll for a headless
// test and returns a restore func. Test-only seam: never call it from
// production code. Cursor position has its own seam, SetPollCursorForTest.
func SetPressEdgeForTest(f func(ebiten.MouseButton) bool) func() {
	prev := pressEdge
	pressEdge = f
	return func() { pressEdge = prev }
}

// PressEdge reports this frame's raw press edge for b through the seam
// above — the same edge Menu.Poll dispatches on, so one scripted click
// drives the popup and the kit's window raise alike (production: raw
// ebiten inpututil, never ebitenui's cached input).
func PressEdge(b ebiten.MouseButton) bool { return pressEdge(b) }

// pollCursor is the menu's cursor read: live off ebiten, NOT ebitenui's
// input.CursorPosition() — that one is a cache refreshed only inside
// UI.Update, which Kit.Update deliberately skips while a menu is open
// (modality), so the cached value freezes at the open anchor and every
// hit test lands 5px above row 0. Headless tests cannot move the OS
// cursor, so the read goes through this indirection.
// SetPollCursorForTest swaps it — tests only; the default is the real poll.
var pollCursor = ebiten.CursorPosition

// SetPollCursorForTest overrides the menu's live cursor poll for a
// headless test and returns a restore func. Test-only seam: never call
// it from production code.
func SetPollCursorForTest(f func() (int, int)) func() {
	prev := pollCursor
	pollCursor = f
	return func() { pollCursor = prev }
}

// PollCursor reads the live cursor through the seam above (production:
// raw ebiten.CursorPosition — the same position Menu.Poll and the kit's
// window raise must agree on; ebitenui's cache is stale by design).
func PollCursor() (int, int) { return pollCursor() }

// Poll runs one frame of menu input: hover from the live cursor, then
// Esc and the mouse buttons (any press dismisses; a press on an enabled
// entry runs it). It reports whether a menu owned the frame. The
// consumed-gesture latch is set by Click/Esc and cleared by exactly one
// place — Menu.TakeBlocked, in Kit.Update (the contract on Menu).
func (m *Menu) Poll() bool {
	if m == nil || !m.open {
		return false
	}
	// One gesture, one action: this frame's press already ran (an entry's
	// Action may have reopened the popup), so a second poll in the same
	// frame — Kit.Update's, after the host's MenuInput pass — must not run
	// an entry again. The consumed-gesture latch is cleared later in the
	// same frame, by Menu.TakeBlocked in Kit.Update.
	if m.blocked {
		return true
	}
	// Live read: ebitenui's cursor cache is stale by design while open.
	x, y := pollCursor()
	pt := image.Pt(x, y)
	m.Hover(pt)
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		m.Esc()
		return true
	}
	if pressEdge(ebiten.MouseButtonLeft) || pressEdge(ebiten.MouseButtonRight) {
		m.Click(pt)
		return true
	}
	return true
}

// ClickPoint is the absolute pointer position of a widget click event —
// the context-menu anchor (the click position IS the cursor, so the menu
// opens where EVE puts it). A test-fired event with no widget falls back
// to the live cursor.
func ClickPoint(a *widget.WidgetMouseButtonClickedEventArgs) image.Point {
	if a != nil && a.Widget != nil && !a.Widget.Rect.Empty() {
		return a.Widget.Rect.Min.Add(image.Pt(a.OffsetX, a.OffsetY))
	}
	x, y := input.CursorPosition()
	return image.Pt(x, y)
}
