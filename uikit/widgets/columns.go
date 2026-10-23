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

// Column layout, column preferences and the header's column chrome.
//
// ONE layout pass owns the columns: applyWidths turns the frozen bases,
// the visible set and the table width into every visible width, and
// layoutCells/headerLayout derive the header buttons, the grip strips and
// the row cell rects from that result. No other site computes a column
// width.
//
// Two policies, kept apart on purpose:
//
//  1. First fit (defaults). With a width and rows on the first fit, the
//     table measures the content once and freezes the result — the old
//     elastic fit: content widths, surplus to the lowest-Priority grow
//     column, Priority shrink toward the header floors. Every default
//     frame is byte-identical to the pre-freeze table. The result IS each
//     column's base, and the bases never re-measure: a cell growing
//     mid-warp or the sort arrow moving to another header ellipsizes
//     instead.
//  2. Every later layout (a table resize, a visibility change, a stored
//     pref, a grip drag) distributes the table width over the frozen bases
//     PROPORTIONALLY: the ratios hold — a column the player made smaller
//     stays smaller and the freed width spreads over the others — so the
//     rows still tile the table exactly. A table narrower than the bases
//     gives width back in Priority order toward the floors, unchanged.
//     The pass itself never writes back, so hiding is a pure display
//     filter and showing the column again restores its frozen width.
//
// A table resize and a grip press adopt the layout they just produced as
// the base (adoptWidths): those are the moments where the act itself IS the
// layout, and a drag must edit the widths the player sees — otherwise the
// fit's surplus or its priority shrink sits outside the base and a grip can
// only move the invisible part, bottoming the column out at floor+surplus.
//
// Interleave rules (the rest of this file):
//   - grips sit only on a boundary two visible columns share; the last
//     visible column has no grip (its right edge is the table's);
//   - the ⋯ button owns ColMenuW at the header's right end: the columns
//     tile the width less that strip, so nothing runs under the dots;
//   - visibility is a display filter: cols and every row's Cells stay
//     positional, and a hidden column keeps its base width.
//   - PlateBand (table.go) is the neighbouring rule BEHIND the rows: a window's
//     artwork band (screens/winheader.go) holds the viewport's fill down to
//     its bottom edge, so no column width moves — the tiling above is
//     unchanged, only the paint under the first rows.

// Column chrome metrics (design units).
const (
	// colGripW is the resize strip at each column's right edge, inside the
	// header strip.
	colGripW = 6
	// ColMenuW is the ⋯ button's strip at the header's right end: the
	// columns tile the table width less it, so no column runs under the
	// dots (and the header labels never paint over them).
	ColMenuW = 22
)

// layoutColumns is the one width computation: every visible column's width
// in a table avail px wide, from the frozen bases. At or above the bases it
// scales them proportionally (the ratios are the contract); a narrower table
// takes width back in Priority order toward the floors. Hidden columns get
// no width and no base of theirs takes part (the display filter). avail <= 0
// (no table width yet) yields the bases themselves.
func layoutColumns(bases []ColWidth, prio []int, hidden []bool, avail float64) []float64 {
	out := make([]float64, len(bases))
	vis := make([]int, 0, len(bases))
	sum := 0.0
	for i := range bases {
		if hidden[i] {
			continue
		}
		vis = append(vis, i)
		sum += bases[i].Width
	}
	switch {
	case len(vis) == 0:
	case avail <= 0 || sum == avail:
		for _, i := range vis {
			out[i] = bases[i].Width
		}
	case sum < avail:
		scale := avail / sum
		for _, i := range vis {
			out[i] = bases[i].Width * scale
		}
	default:
		sub := make([]ColWidth, len(vis))
		p := make([]int, len(vis))
		for k, i := range vis {
			sub[k], p[k] = bases[i], prio[i]
		}
		sub = fitColumns(sub, p, -1, avail) // -1: no grow column after the freeze
		for k, i := range vis {
			out[i] = sub[k].Width
		}
	}
	return out
}

// colOffsets is the one tiling rule: column i's rounded right edge, from
// the table's left edge. Header buttons, grip strips and row cells all
// anchor on it, so a grip sits exactly on the boundary it drags.
func colOffsets(widths []float64) []int {
	out := make([]int, len(widths))
	acc := 0.0
	for i, w := range widths {
		acc += w
		out[i] = int(math.Round(acc))
	}
	return out
}

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
	t.prefs = make(map[string]ColumnPref, len(p))
	for k, v := range p {
		t.prefs[k] = v
	}
	if t.frozen {
		t.applyPrefs()
		t.applyWidths()
	}
}

// Prefs reads the table's column state back for the sidecar. A table that
// has not been fitted yet (no rows, never laid out) reports the stored set
// unchanged — an empty list keeps its column state instead of losing it.
//
// The stored widths are the player's: a drag records them (storeWidths), a
// window resize does not — a resize scales the live layout, it does not
// rewrite what the player chose. A column with no stored width reports the
// frozen default, so a table that was never touched still hands the sidecar
// a full set.
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
		p := t.prefs[c.Key]
		p.Off = t.hidden[i]
		if p.Width == 0 && s > 0 {
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
	w, h := max(theme.Px(ColMenuW), 3), max(theme.Px(TableHeaderH), 3)
	dot, gap := max(theme.Px(SplitGripDot), 1), theme.Px(SplitGripGap)
	img := ebiten.NewImage(w, h)
	x, y := (w-(3*dot+2*gap))/2, (h-dot)/2
	for i := 0; i < 3; i++ {
		at := x + i*(dot+gap)
		theme.FillRect(img, image.Rect(at, y, at+dot, y+dot), c)
	}
	return eimage.NewFixedNineSlice(img)
}

// headerLayout places the sort row across the strip, a grip on every
// boundary two visible columns share, and the ⋯ button in its own strip
// at the header's right end (the columns tile up to it, never under it).
// It anchors on colOffsets, the same cumulative rounding layoutCells uses,
// so a grip sits exactly on the boundary it drags.
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
	menu := image.Rect(rect.Max.X-theme.Px(ColMenuW), rect.Min.Y, rect.Max.X, rect.Max.Y)
	ws[1+len(t.cols)].SetLocation(menu)
	off := colOffsets(t.widths)
	last := -1
	for i := range t.cols {
		if !t.hidden[i] {
			last = i
		}
	}
	prev := rect.Min.X
	for i := range t.cols {
		if i >= len(off) {
			ws[1+i].SetLocation(image.Rectangle{})
			continue
		}
		next := rect.Min.X + off[i]
		// Only a boundary two visible columns share can be dragged: the
		// last visible column's right edge is the table's own (the fit's),
		// and a hidden column owns no edge at all. A column no wider than
		// its grip keeps no strip (there would be nothing to drag).
		grip := image.Rectangle{}
		if !t.hidden[i] && i != last && next-prev > theme.Px(colGripW) {
			grip = image.Rect(next-theme.Px(colGripW), rect.Min.Y, next, rect.Max.Y)
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
// The press adopts the laid-out widths as the frozen base (adoptWidths), so
// the drag edits exactly what the player sees.
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
		t.adoptWidths()
		t.grip, t.gripX, t.gripFrom = i, cursor.X, t.base[i].Width
	}
}

// adoptWidths makes the current layout the frozen base. It runs where the
// act itself IS the layout: a table resize (the new width is the input, and
// later hides and drags continue from what was on screen) and a grip press
// (the drag must edit the widths the player sees — otherwise the fit's
// surplus, or its priority shrink, sits outside the base and the grip can
// only move the invisible part, bottoming the column out at floor+surplus).
// A content change or a visibility toggle never adopts: a hidden column
// keeps its base width, so showing it again restores it.
func (t *Table) adoptWidths() {
	if len(t.widths) != len(t.base) {
		return
	}
	for i := range t.base {
		if !t.hidden[i] && t.widths[i] > 0 {
			t.base[i].Width = t.widths[i]
		}
	}
}

// storeWidths records the player's tiling as the stored column widths (in
// design units), so a drag persists while a resize — which also adopts the
// tiling — never rewrites what the player chose.
func (t *Table) storeWidths() {
	s := theme.Scale()
	if t.prefsID == "" || s <= 0 {
		return
	}
	if t.prefs == nil {
		t.prefs = map[string]ColumnPref{}
	}
	for i, c := range t.cols {
		if t.hidden[i] {
			continue
		}
		p := t.prefs[c.Key]
		p.Width, p.Off = t.base[i].Width/s, false
		t.prefs[c.Key] = p
	}
}

// dragTo sets the grabbed column's frozen width from the cursor delta: the
// boundary under the pointer follows it, never below either column's header
// floor and with no ceiling.
func (t *Table) dragTo(cursor image.Point) {
	t.setColWidth(t.grip, t.gripFrom+float64(cursor.X-t.gripX))
}

// setColWidth writes one column's frozen width. The delta is traded with
// the NEXT visible column — the one whose left edge is the boundary the
// strip sits on, so the grabbed edge moves with the pointer — and both
// sides stop at their header floor, so the row of columns keeps tiling the
// table exactly and the fit has nothing to redistribute.
func (t *Table) setColWidth(i int, w float64) {
	if i < 0 || i >= len(t.base) || i >= len(t.cols) {
		return
	}
	j := t.nextVisible(i)
	if j < 0 {
		return // the table's right edge is the fit's, not a drag's
	}
	d := w - t.base[i].Width
	d = math.Max(d, t.base[i].Min-t.base[i].Width) // the dragged column's floor
	d = math.Min(d, t.base[j].Width-t.base[j].Min) // the partner's floor
	if d == 0 {
		return
	}
	t.base[i].Width += d
	t.base[j].Width -= d
	t.applyWidths()
	t.storeWidths()
}

// nextVisible is the visible column after i (-1 when none): the column on
// the far side of the boundary i's strip drags.
func (t *Table) nextVisible(i int) int {
	for j := i + 1; j < len(t.cols); j++ {
		if !t.hidden[j] {
			return j
		}
	}
	return -1
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
