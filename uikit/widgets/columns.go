package widgets

import (
	"image"
	"image/color"
	"math"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// Column preferences and the header's column chrome.
//
// A table measures its columns ONCE, on the first fit, and freezes the
// result (Table.ensureFrozen): from then on a cell growing — the
// overview's Velocity mid-warp, the sort arrow moving to another header —
// can never move a pixel. Content that no longer fits ellipsizes instead.
// The frozen widths are what the ⋯ menu and the drag grips change, and
// what the layout sidecar persists.
//
// Display filter: hiding a column filters layout, drawing and hit tests by
// column index. cols and every row's Cells stay positional, so hide/show
// never re-indexes a row (the surfaces' index pins keep holding).

// Column chrome metrics (design units).
const (
	// colGripW is the resize strip at each column's right edge, inside the
	// header strip.
	colGripW = 6
	// colMenuW is the ⋯ button at the header's right end.
	colMenuW = 22
)

// ColumnPref is one table column's persisted state, in DESIGN units (the
// sidecar convention — theme.Px scales it back): Width is the frozen
// column width (0 = the measured default), Off hides the column.
type ColumnPref struct {
	Width float64 `json:"width,omitempty"`
	Off   bool    `json:"off,omitempty"`
}

// PrefsID tags a table for the layout sidecar: tables that share an id
// share one stored column set (the inventory's three hold tables share
// their columns, so they share the widths the player dragged).
func PrefsID(id string) TableOption {
	return func(t *Table) { t.prefsID = id }
}

// PrefsID is the table's layout-sidecar key ("" = not persisted).
func (t *Table) PrefsID() string { return t.prefsID }

// SetPrefs applies the stored column state. Call it after construction
// and before the first rows: the widths overlay the frozen measurement.
func (t *Table) SetPrefs(p map[string]ColumnPref) {
	t.prefs = p
	if t.frozen {
		t.applyPrefs()
		t.applyWidths()
	}
}

// Prefs reads the table's column state back for the sidecar. A table that
// has not been fitted yet (no rows, never laid out) reports the stored set
// unchanged — an empty list keeps its column state instead of losing it.
func (t *Table) Prefs() map[string]ColumnPref {
	if t.prefsID == "" {
		return nil
	}
	if !t.frozen {
		return t.prefs
	}
	s := theme.Scale()
	out := make(map[string]ColumnPref, len(t.cols))
	for i, c := range t.cols {
		p := ColumnPref{Off: t.hidden[i]}
		if s > 0 {
			p.Width = t.base[i].Width / s
		}
		out[c.Key] = p
	}
	return out
}

// ResetPrefs drops the frozen measurement and every stored column
// preference: the next fit measures the content again ("Reset window
// layout").
func (t *Table) ResetPrefs() {
	t.prefs = nil
	t.frozen = false
	for i := range t.hidden {
		t.hidden[i] = false
	}
	t.applyWidths()
}

// applyPrefs overlays the stored state on the frozen base: the stored
// width replaces the measured one (never below the header floor) and the
// stored flags hide columns.
func (t *Table) applyPrefs() {
	for i, c := range t.cols {
		p, ok := t.prefs[c.Key]
		if !ok {
			continue
		}
		if p.Width > 0 {
			t.base[i].Width = math.Max(p.Width*theme.Scale(), t.base[i].Min)
		}
		t.hidden[i] = p.Off
	}
	t.keepOneVisible()
}

// visibleCols counts the columns the display filter shows.
func (t *Table) visibleCols() int {
	n := 0
	for _, h := range t.hidden {
		if !h {
			n++
		}
	}
	return n
}

// keepOneVisible unhides the first column when the state hides all of
// them: the menu disables that entry, but a hand-edited sidecar could
// still ask for it and a table with no column shows nothing at all.
func (t *Table) keepOneVisible() {
	if t.visibleCols() > 0 || len(t.hidden) == 0 {
		return
	}
	t.hidden[0] = false
}

// setColumnVisible shows or hides column i (layout, draw and hit tests —
// the positional data never changes). Hiding the last visible column is a
// no-op: a table always shows at least one.
func (t *Table) setColumnVisible(i int, show bool) {
	if i < 0 || i >= len(t.cols) || t.hidden[i] != show {
		return
	}
	if !show && t.visibleCols() <= 1 {
		return
	}
	t.hidden[i] = !show
	t.applyWidths()
}

// openColumnMenu opens the header's ⋯ menu: one row per column carrying
// its checked state. A row toggles that column and reopens the menu at the
// same point — the menu's own "an action may open another menu" contract
// (menu.go, Menu.Click closes before it runs the action).
func (t *Table) openColumnMenu(at image.Point) {
	entries := make([]MenuEntry, len(t.cols))
	for i, c := range t.cols {
		i := i
		entries[i] = MenuEntry{
			Label:   c.Title,
			Checked: !t.hidden[i],
			// The last visible column has to stay: its row only reports.
			Disabled: !t.hidden[i] && t.visibleCols() == 1,
			Action: func() {
				t.setColumnVisible(i, t.hidden[i])
				t.openColumnMenu(at)
			},
		}
	}
	t.as.Do(OpenMenu{Entries: entries, At: at})
}

// ---------------------------------------------------------------------------
// Header chrome: the resize grips and the ⋯ button
// ---------------------------------------------------------------------------

// buildColumnChrome adds the header's overlay chrome. Both are buttons
// that elevate a blocking input layer of their own (WidgetOpts.ElevateLayer),
// so the strip claims the press and the sort button underneath never sees
// the click: a press on the strip resizes, a press anywhere else in the
// header sorts.
func (t *Table) buildColumnChrome() {
	p := t.th.Pal()
	states := func(idle, hover *eimage.NineSlice) *widget.ButtonImage {
		return &widget.ButtonImage{Idle: idle, Hover: hover, Pressed: hover, PressedHover: hover, Disabled: idle}
	}
	for range t.cols {
		b := widget.NewButton(
			widget.ButtonOpts.Image(states(colHairline(theme.NRGBA(p.Border, 160)), colHairline(theme.NRGBA(p.Accent, 220)))),
			// The strip owns its own (blocking) input layer over the sort
			// button tiling underneath it: a press there never sorts.
			widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.ElevateLayer(true)),
			// The drag rides Poll: a press only arms it, so the click has
			// nothing to do — the button exists to own the strip.
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {}),
		)
		t.grips = append(t.grips, b)
		t.header.AddChild(b)
	}
	t.menuBtn = widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:         colDots(p.Dim),
			Hover:        colDots(p.Text),
			Pressed:      colDots(p.Accent),
			PressedHover: colDots(p.Accent),
			Disabled:     colDots(p.Dim),
		}),
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.ElevateLayer(true)),
		widget.ButtonOpts.ClickedHandler(func(a *widget.ButtonClickedEventArgs) {
			// The popup anchors at the click itself (the row menus' rule).
			at := image.Pt(a.Button.GetWidget().Rect.Min.X+a.OffsetX, a.Button.GetWidget().Rect.Min.Y+a.OffsetY)
			t.openColumnMenu(at)
		}),
	)
	t.header.AddChild(t.menuBtn)
}

// colHairline is a grip's background: a 1 px column boundary at the strip's
// right edge (the edge it drags), stretched to the header height.
func colHairline(c color.Color) *eimage.NineSlice {
	w := max(theme.Px(colGripW), 3)
	b := max(theme.Px(1), 1)
	img := ebiten.NewImage(w, 3)
	theme.FillRect(img, image.Rect(w-b, 0, w, 3), c)
	return eimage.NewNineSlice(img, [3]int{w - b, b, 0}, [3]int{1, 1, 1})
}

// colDots is the ⋯ glyph: three runtime-drawn dots on the split grip's dot
// language (2 px dots, 2 px apart) — never a font glyph, the interface has
// no cmap probe for U+22EF. The image is the button's exact size, so the
// fixed nine-slice fills it.
func colDots(c color.Color) *eimage.NineSlice {
	w, h := max(theme.Px(colMenuW), 3), max(theme.Px(TableHeaderH), 3)
	dot, gap := max(theme.Px(SplitGripDot), 1), theme.Px(SplitGripGap)
	img := ebiten.NewImage(w, h)
	x, y := (w-(3*dot+2*gap))/2, (h-dot)/2
	for i := 0; i < 3; i++ {
		at := x + i*(dot+gap)
		theme.FillRect(img, image.Rect(at, y, at+dot, y+dot), c)
	}
	return eimage.NewFixedNineSlice(img)
}

// headerLayout places the sort row across the strip, a grip at every
// visible column's right edge (the last one stops at the ⋯ button, which
// owns the strip's right end) and the menu button there. It reuses
// layoutCells' cumulative rounding, so a grip sits exactly on the boundary
// it drags.
type headerLayout struct{ t *Table }

// PreferredSize is the sort row's own: the chrome rides over it and never
// widens the header.
func (h headerLayout) PreferredSize(ws []widget.PreferredSizeLocateableWidget) (int, int) {
	if len(ws) == 0 {
		return 0, 0
	}
	return ws[0].PreferredSize()
}

// Layout implements widget.Layouter.
func (h headerLayout) Layout(ws []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	t := h.t
	if len(ws) > 0 {
		ws[0].SetLocation(rect)
	}
	if len(ws) < len(t.cols)+2 {
		return
	}
	menu := image.Rect(rect.Max.X-theme.Px(colMenuW), rect.Min.Y, rect.Max.X, rect.Max.Y)
	ws[1+len(t.cols)].SetLocation(menu)
	last := -1
	for i := range t.cols {
		if !t.hidden[i] {
			last = i
		}
	}
	acc, prev := 0.0, rect.Min.X
	for i := range t.cols {
		if i >= len(t.widths) {
			ws[1+i].SetLocation(image.Rectangle{})
			continue
		}
		acc += t.widths[i]
		next := rect.Min.X + int(math.Round(acc))
		grip := image.Rectangle{}
		edge := next
		if i == last {
			edge = min(edge, menu.Min.X)
		}
		// A column no wider than its grip keeps no strip (there would be
		// nothing to drag).
		if !t.hidden[i] && edge-prev > theme.Px(colGripW) {
			grip = image.Rect(edge-theme.Px(colGripW), rect.Min.Y, edge, rect.Max.Y)
		}
		ws[1+i].SetLocation(grip)
		prev = next
	}
}

// ---------------------------------------------------------------------------
// Grip dragging
// ---------------------------------------------------------------------------

// Poll advances one frame of column dragging from the live cursor and the
// left button (the split seam's pattern — Table.Sync calls it, so a drag
// costs one cursor read per frame).
func (t *Table) Poll() {
	x, y := PollCursor()
	t.Step(image.Pt(x, y), PressEdge(ebiten.MouseButtonLeft), splitHeld())
}

// Step is Poll's testable core: a rising press on a grip strip grabs that
// column's width, the held drag follows the cursor, the release drops it.
func (t *Table) Step(cursor image.Point, rising, pressed bool) {
	if t.grip >= 0 {
		if !pressed {
			t.grip = -1
			return
		}
		t.dragTo(cursor)
		t.setGripCursor()
		return
	}
	i := t.gripAt(cursor)
	if i < 0 {
		return
	}
	t.setGripCursor()
	if rising {
		t.grip, t.gripX, t.gripFrom = i, cursor.X, t.base[i].Width
	}
}

// dragTo sets the grabbed column's frozen width from the cursor delta: its
// right edge follows the pointer, never below the header floor and with no
// ceiling.
func (t *Table) dragTo(cursor image.Point) {
	t.setColWidth(t.grip, t.gripFrom+float64(cursor.X-t.gripX))
}

// setColWidth writes one column's frozen width. The delta is traded with
// the next-most-important visible column, so the row of columns keeps
// tiling the table exactly and the fit has nothing to redistribute — a
// drag is what the player sees, whichever column they grabbed.
func (t *Table) setColWidth(i int, w float64) {
	if i < 0 || i >= len(t.base) || i >= len(t.cols) {
		return
	}
	w = math.Max(w, t.base[i].Min)
	d := w - t.base[i].Width
	if d == 0 {
		return
	}
	t.base[i].Width = w
	if j := t.otherVisible(i); j >= 0 {
		t.base[j].Width = math.Max(t.base[j].Width-d, t.base[j].Min)
	}
	t.applyWidths()
}

// otherVisible is the visible column a drag trades width with: the least
// important one besides i (the fit's grow column, or the runner-up when i
// is the grow column).
func (t *Table) otherVisible(i int) int {
	best := -1
	for j := range t.cols {
		if j == i || t.hidden[j] {
			continue
		}
		if best < 0 || t.cols[j].Priority < t.cols[best].Priority {
			best = j
		}
	}
	return best
}

// gripAt is the grip strip under p, or -1. Each grip owns a blocking input
// layer, and ActiveFor walks the stack: a strip under another window (or
// under the modal menu) is inert — the split seam's idiom.
func (t *Table) gripAt(p image.Point) int {
	for i, g := range t.grips {
		w := g.GetWidget()
		if w.Rect.Empty() || !p.In(w.Rect) {
			continue
		}
		if !w.EffectiveInputLayer().ActiveFor(p.X, p.Y, input.LayerEventTypeMouseButton) {
			continue
		}
		return i
	}
	return -1
}

// setGripCursor claims the resize cursor (the split row's EW shape) while a
// grip is under the pointer or dragged.
func (t *Table) setGripCursor() { ClaimCursor(ebiten.CursorShapeEWResize) }
