package uikit

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"sort"
	"time"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Table layout constants (derived from the legacy ui padding scale).
const (
	// tableHeaderH is the height of the sort-button header strip.
	tableHeaderH = 30
	// headerSpace is the gap between the header strip and the rows.
	headerSpace = 4
	// cellPad is the horizontal padding inside every column cell.
	cellPad = 5
	// entryPadV is the vertical padding inside a row.
	entryPadV = 5
	// selectionAlpha is the accent tint alpha of the selected row.
	selectionAlpha = 64
	// wheelRows is how many rows one mouse-wheel notch scrolls.
	wheelRows = 3
)

// doubleClickWindow is the maximum gap between two row activations that
// still counts as a double click (activate).
const doubleClickWindow = 350 * time.Millisecond

// Column describes one table column: its sort key/title, its preferred
// (content) width and shrink floor, its shrink priority when space is
// short (higher drops first), and its text alignment.
type Column struct {
	Key        string  // sort key passed to SetSort/SetSortFunc
	Title      string  // header label
	Width      float64 // preferred (content) width; 0 = measure the content
	MinWidth   float64 // never shrink below this (the title's own width is an implicit floor)
	Priority   int     // shrink order when over space (higher drops first)
	AlignRight bool    // numeric columns: fixed right-aligned anchors
}

// TableRow is one displayed row: one cell string per Column, an optional
// background tint (zero = none) and an opaque payload handed to the row
// callbacks (e.g. sim.Info, content.ItemID). Icons, when set, runs parallel
// to Cells: an icon name per cell ("" = none) drawn in a fixed box to the
// left of that cell's text. Right-aligned columns ignore it — numeric
// anchors stay exact — so icons belong in the left-aligned cells.
type TableRow struct {
	Cells    []string
	Icons    []string
	IconTint color.RGBA // icon palette role; zero = ColText
	Tint     color.RGBA
	Data     any
}

// iconOf returns row cell i's icon name ("" when the row carries none).
func iconOf(row TableRow, i int) string {
	if i < 0 || i >= len(row.Icons) {
		return ""
	}
	return row.Icons[i]
}

// Table is a sortable, scrollable column table: a header row of sort
// buttons over a widget.ScrollContainer of per-row widget.Containers.
//
// Unlike widget.List it owns its scroll instance (ScrollTop is readable),
// every row is a real container (full per-row background tinting) and the
// columns are real fixed-width labels (right-aligned cells share exact
// anchors). Events in ebitenui are deferred to the next UI.Update, so
// handlers registered here go live on the first update pump.
type Table struct {
	env         Env
	cols        []Column
	face        *text.Face
	measure     func(string) float64 // cell width, regular weight
	headMeasure func(string) float64 // header width, emphasis weight
	rowH        float64

	root    *widget.Container
	header  *widget.Container
	hdrBtn  []*widget.Button
	hdrIdle *widget.ButtonImage
	hdrOn   *widget.ButtonImage
	scroll  *widget.ScrollContainer
	body    *widget.Container

	rows       []TableRow // display order (sorted)
	rawRows    []TableRow // cell/icon content last applied to the widgets
	texts      [][]*widget.Text
	cells      [][]*tableCell // per-cell widget pair (nil for right-aligned)
	rowCts     []*widget.Container
	applied    []color.RGBA // background last painted per row widget
	widths     []float64    // fitted column widths
	fittedFor  float64      // available width the fit was computed for
	sortKey    string
	sortAsc    bool
	sortFn     func(key string, asc bool, rows []TableRow)
	onSelect   func(row TableRow)
	onActivate func(row TableRow)
	onMenu     func(row TableRow, at image.Point)

	selIdx  int // selected row index, -1 = none
	selData any // Data of the selected row (identity across rebuilds)

	last    TableRow
	hasLast bool
	lastAt  time.Time
}

// NewTable builds a table with the given columns over the shared
// environment (faces and palette). Rows arrive via SetRows; embed it in a
// window with Container().
func NewTable(env Env, cols []Column) *Table {
	p := env.Pal
	face := env.Face(StepSmall)
	// Headers draw in the emphasis weight (Turret Road Bold), so their
	// widths are measured with it too — the fit and the drawn label have
	// to agree or the header overflows its column.
	headFace := env.FaceBold(StepSmall)
	t := &Table{
		env:    env,
		cols:   append([]Column(nil), cols...),
		face:   face,
		selIdx: -1,
	}
	t.measure = func(s string) float64 {
		w, _ := text.Measure(s, *face, 0)
		return w
	}
	t.headMeasure = func(s string) float64 {
		w, _ := text.Measure(s, *headFace, 0)
		return w
	}
	m := (*face).Metrics()
	t.rowH = math.Ceil(m.HAscent+m.HDescent) + 2*entryPadV

	t.hdrIdle = &widget.ButtonImage{
		Idle:         bordered(solid(p.HeadBg), nrgba(p.Border, 160), 1),
		Hover:        bordered(nrgba(p.Hover, 18), nrgba(p.Accent, 200), 1),
		Pressed:      bordered(nrgba(p.Hover, 45), nrgba(p.Border, 220), 1),
		PressedHover: bordered(nrgba(p.Hover, 45), nrgba(p.Accent, 200), 1),
		Disabled:     bordered(nrgba(p.Cell, 140), nrgba(p.Border, 110), 1),
	}
	t.hdrOn = &widget.ButtonImage{
		Idle:         bordered(nrgba(p.Accent, 36), nrgba(p.Accent, 200), 1),
		Hover:        bordered(nrgba(p.Accent, 60), nrgba(p.Accent, 220), 1),
		Pressed:      bordered(nrgba(p.Accent, 80), nrgba(p.Accent, 220), 1),
		PressedHover: bordered(nrgba(p.Accent, 80), nrgba(p.Accent, 220), 1),
		Disabled:     bordered(nrgba(p.Cell, 140), nrgba(p.Border, 110), 1),
	}

	// Header: one sort button per column; applyWidths pins every button
	// to its fitted column so headers stay aligned with the row cells.
	t.header = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(eimage.NewNineSliceColor(solid(p.HeadBg))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)
	for i := range t.cols {
		i := i
		c := t.cols[i]
		align := widget.TextPositionStart
		if c.AlignRight {
			align = widget.TextPositionEnd
		}
		btn := widget.NewButton(
			widget.ButtonOpts.TextLabel(c.Title),
			widget.ButtonOpts.TextFace(headFace),
			widget.ButtonOpts.TextColor(&widget.ButtonTextColor{
				Idle:     solid(p.Text),
				Hover:    solid(p.Text),
				Pressed:  solid(p.Text),
				Disabled: nrgba(p.Dim, 160),
			}),
			widget.ButtonOpts.TextPosition(align, widget.TextPositionCenter),
			widget.ButtonOpts.TextPadding(&widget.Insets{
				Left: cellPad, Right: cellPad, Top: 4, Bottom: 4,
			}),
			widget.ButtonOpts.Image(t.hdrIdle),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, tableHeaderH),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
				t.headerClicked(i)
			}),
		)
		t.hdrBtn = append(t.hdrBtn, btn)
		t.header.AddChild(btn)
	}

	// Rows: one container per row inside the scroll content.
	t.body = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)
	t.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(t.body),
		// Columns are fitted to the view width, so the content stretches
		// to fill it instead of scrolling horizontally at rest.
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle:     bordered(solid(p.BG), nrgba(p.Border, 160), 1),
			Disabled: bordered(nrgba(p.Cell, 140), nrgba(p.Border, 110), 1),
			Mask:     eimage.NewNineSliceColor(solid(p.BG)),
		}),
	)
	// Wheel scrolls the viewport; ScrollTop is a 0..1 fraction of the
	// content overflow, so the step is scaled by the measured overflow.
	t.scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			t.scrollBy(ev.Y)
		}
	})

	// Root: header row (fixed) over the scroll row (fills the rest).
	// A grid keeps the viewport bounded by the window — the scroll
	// content's own preferred height must never leak into it.
	t.root = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, headerSpace),
		)),
	)
	t.root.AddChild(t.header, t.scroll)
	return t
}

// Container returns the table's widget tree for embedding in a window or
// the root container. Give it the full cell of a stretched grid row (or
// an AnchorLayoutData with both stretches) so the viewport is bounded.
func (t *Table) Container() widget.Containerer { return t.root }

// SelectData selects the row carrying the given Data identity (nil clears
// the selection) and refreshes row highlights. For selection that
// originates outside the table — world clicks, keyboard cycles.
func (t *Table) SelectData(v any) {
	t.selIdx, t.selData = -1, nil
	if v != nil {
		for i, r := range t.rows {
			if sameData(r.Data, v) {
				t.selIdx, t.selData = i, v
				break
			}
		}
	}
	t.updateRows()
}

// OnSelect registers the row-click callback (fires on every click).
func (t *Table) OnSelect(fn func(row TableRow)) { t.onSelect = fn }

// Selected returns the currently selected row in display order and
// whether there is one. It reports the reconciled state: a row whose
// Data vanished from the table since the click reports no selection, so
// owners caching a selection across Syncs can revalidate against it.
func (t *Table) Selected() (TableRow, bool) {
	if t.selIdx < 0 || t.selIdx >= len(t.rows) {
		return TableRow{}, false
	}
	return t.rows[t.selIdx], true
}

// OnActivate registers the row double-click callback: a second click on
// the same row within doubleClickWindow of the first.
func (t *Table) OnActivate(fn func(row TableRow)) { t.onActivate = fn }

// OnMenu registers the row right-click callback: it receives the row and
// the click position in screen space (the context-menu anchor). Rows are
// hit at row granularity — cells are not individually hit-testable.
func (t *Table) OnMenu(fn func(row TableRow, at image.Point)) { t.onMenu = fn }

// SetSortFunc overrides how rows are ordered for a column key: fn must
// order rows in place (stable). The default orders lexicographically by
// the sort column's cell. Passing nil restores the default.
func (t *Table) SetSortFunc(fn func(key string, asc bool, rows []TableRow)) {
	t.sortFn = fn
}

// SetSort activates the given sort column and direction: it re-orders the
// current rows, moves the ↑/↓ indicator and re-fits the header widths.
func (t *Table) SetSort(key string, asc bool) {
	t.sortKey, t.sortAsc = key, asc
	t.applySort()
	t.reconcileSelection()
	t.updateRows()
	t.applyWidths()
}

// SetRows replaces the table's rows (a copy), applies the active sort,
// keeps the selection when its row still exists and rebuilds or updates
// the row widgets. The columns are re-fitted only when the row set, a
// cell's text or an icon actually changed (or on the first fit): an
// identical SetRows is a visual no-op and costs no measuring or layout.
func (t *Table) SetRows(rows []TableRow) {
	t.rows = append([]TableRow(nil), rows...)
	t.applySort()
	t.reconcileSelection()
	// t.widths is nil until the first fit — that one always lays out,
	// whatever the rows are (the header buttons need their boxes).
	if t.updateRows() || t.widths == nil {
		t.applyWidths()
	}
}

// Sync re-fits the columns when the table's width changed (window
// resize/drag) and re-pins the scroll offset to the current overflow
// (viewport growth can make a scrolled list fit again). Owners call it
// once per frame; it is cheap when nothing changed.
func (t *Table) Sync() {
	t.clampScroll()
	avail := float64(t.root.GetWidget().Rect.Dx())
	if avail > 0 && avail != t.fittedFor {
		t.applyWidths()
	}
}

// Refit re-measures and re-applies the column widths immediately (for
// owners that resize the table outside the normal frame pump).
func (t *Table) Refit() { t.applyWidths() }

// headerClicked toggles the sort column/direction on a header click.
func (t *Table) headerClicked(i int) {
	key := t.cols[i].Key
	if t.sortKey == key {
		t.SetSort(key, !t.sortAsc)
		return
	}
	t.SetSort(key, true)
}

// applySort orders t.rows by the active sort (stable).
func (t *Table) applySort() {
	if t.sortKey == "" {
		return
	}
	idx := t.colIndex(t.sortKey)
	if idx < 0 {
		return
	}
	if t.sortFn != nil {
		t.sortFn(t.sortKey, t.sortAsc, t.rows)
		return
	}
	sort.SliceStable(t.rows, func(a, b int) bool {
		x, y := cellOf(t.rows[a], idx), cellOf(t.rows[b], idx)
		if t.sortAsc {
			return x < y
		}
		return y < x
	})
}

// colIndex returns the column index of a sort key (-1 if unknown).
func (t *Table) colIndex(key string) int {
	for i, c := range t.cols {
		if c.Key == key {
			return i
		}
	}
	return -1
}

// reconcileSelection re-resolves the selection after a reorder/rebuild:
// Data is the row's stable identity, index only as long as Data is nil.
func (t *Table) reconcileSelection() {
	if t.selIdx < 0 {
		return
	}
	if t.selData != nil {
		t.selIdx = -1
		for i, r := range t.rows {
			if sameData(r.Data, t.selData) {
				t.selIdx = i
				return
			}
		}
		t.selData = nil
		return
	}
	if t.selIdx >= len(t.rows) {
		t.selIdx = -1
	}
}

// updateRows creates the row widgets when the row count changed and
// refreshes cell text and tints in place otherwise (diff-friendly). It
// reports whether any cell text or icon changed — the input the column
// refit needs; tints and selection repaint through updateBg/updateIcon
// without touching the layout.
func (t *Table) updateRows() bool {
	changed := len(t.rowCts) != len(t.rows)
	if changed {
		t.body.RemoveChildren()
		t.rowCts, t.texts, t.cells, t.applied, t.rawRows = nil, nil, nil, nil, nil
		for i := range t.rows {
			ctr := t.newRow(i)
			t.rowCts = append(t.rowCts, ctr)
			t.body.AddChild(ctr)
		}
	}
	for i, row := range t.rows {
		if i >= len(t.rawRows) {
			t.rawRows = append(t.rawRows, TableRow{})
		}
		// Diff against the raw cells/icons last applied (never against
		// the widget labels: those hold the ellipsized form, so a
		// truncated cell would read as "changed" forever).
		prev := &t.rawRows[i]
		if !sameCellsIcons(*prev, row, len(t.cols)) {
			changed = true
			*prev = TableRow{
				Cells: append([]string(nil), row.Cells...),
				Icons: append([]string(nil), row.Icons...),
			}
			for j := range t.cols {
				cell := cellOf(row, j)
				if tx := t.texts[i][j]; tx.Label != cell {
					// Raw cell; applyWidths ellipsizes it to the fitted column.
					tx.Label = cell
				}
			}
		}
		t.updateIcon(i)
		t.updateBg(i)
	}
	// Row count changed → the content may no longer overflow: drop any
	// leftover scroll offset now, before the next render positions rows.
	t.clampScroll()
	return changed
}

// sameCellsIcons reports whether row's cell text and icons across the
// first n columns match the snapshot (an empty snapshot only matches an
// all-empty row).
func sameCellsIcons(applied, row TableRow, n int) bool {
	for j := 0; j < n; j++ {
		if cellOf(row, j) != cellOf(applied, j) || iconOf(row, j) != iconOf(applied, j) {
			return false
		}
	}
	return true
}

// newRow builds one clickable row container with one cell widget per
// column. The click handler resolves the row by index at fire time, so
// deferred events always hit the currently displayed row (widgets are
// positional: row i's container never moves).
func (t *Table) newRow(i int) *widget.Container {
	ctr := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(0),
		)),
		widget.ContainerOpts.WidgetOpts(
			// Full width: the row background covers the whole strip even
			// when the columns underfill it.
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MouseButtonClickedHandler(func(args *widget.WidgetMouseButtonClickedEventArgs) {
				switch args.Button {
				case ebiten.MouseButtonLeft:
					t.rowClicked(i)
				case ebiten.MouseButtonRight:
					t.rowMenu(i, ClickPoint(args))
				}
			}),
		),
	)
	var texts []*widget.Text
	var cells []*tableCell
	for _, c := range t.cols {
		// Left-aligned cells are icon cells: a fixed icon box then the
		// label, both inside the column box. Right-aligned cells stay
		// plain text so their numeric anchors never move — and they keep
		// the horizontal padding themselves, while an icon cell's lives
		// on its container.
		align, pad := widget.TextPositionStart, widget.Insets{
			Top: entryPadV, Bottom: entryPadV,
		}
		if c.AlignRight {
			align = widget.TextPositionEnd
			pad = widget.Insets{Left: cellPad, Right: cellPad, Top: entryPadV, Bottom: entryPadV}
		}
		tx := widget.NewText(
			widget.TextOpts.Text("", t.face, solid(t.env.Pal.Text)),
			widget.TextOpts.Position(align, widget.TextPositionCenter),
			widget.TextOpts.Padding(&pad),
			widget.TextOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, int(t.rowH)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		if c.AlignRight {
			texts = append(texts, tx)
			cells = append(cells, nil)
			ctr.AddChild(tx)
			continue
		}
		cell := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(IconGap),
				widget.RowLayoutOpts.Padding(&widget.Insets{
					Left: cellPad, Right: cellPad, Top: entryPadV, Bottom: entryPadV,
				}),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, int(t.rowH)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		box := newIconBox(IconInline)
		cell.AddChild(box.Widget(), tx)
		texts = append(texts, tx)
		cells = append(cells, &tableCell{ctr: cell, box: box})
		ctr.AddChild(cell)
	}
	t.texts = append(t.texts, texts)
	t.cells = append(t.cells, cells)
	t.applied = append(t.applied, color.RGBA{})
	return ctr
}

// tableCell pairs a left-aligned cell's container with its icon box.
type tableCell struct {
	ctr *widget.Container
	box *iconBox
}

// cellRoot returns the widget that owns row r column j's box: the icon
// cell container for left-aligned columns, the text itself otherwise. The
// anchor (MinWidth/fitted width) is pinned on this widget.
func (t *Table) cellRoot(r, j int) *widget.Widget {
	if r < 0 || r >= len(t.cells) || j < 0 || j >= len(t.cells[r]) {
		return nil
	}
	if c := t.cells[r][j]; c != nil {
		return c.ctr.GetWidget()
	}
	return t.texts[r][j].GetWidget()
}

// rowWidget returns the i-th row container, or nil.
func (t *Table) rowWidget(i int) *widget.Container {
	if i < 0 || i >= len(t.rowCts) {
		return nil
	}
	return t.rowCts[i]
}

// rowClicked selects the row and, on a repeat click inside the
// double-click window, also activates it (the spike's timing state —
// ebitenui has no double-click event).
func (t *Table) rowClicked(i int) {
	if i < 0 || i >= len(t.rows) {
		return
	}
	row := t.rows[i]
	prev := t.selIdx
	t.selIdx, t.selData = i, row.Data
	if prev != i {
		if prev >= 0 {
			t.updateBg(prev)
			t.updateIcon(prev)
		}
		t.updateBg(i)
		t.updateIcon(i)
	}
	if t.onSelect != nil {
		t.onSelect(row)
	}
	now := time.Now()
	if t.hasLast && sameRow(t.last, row) && now.Sub(t.lastAt) < doubleClickWindow {
		if t.onActivate != nil {
			t.onActivate(row)
		}
	}
	t.last, t.hasLast, t.lastAt = row, true, now
}

// rowMenu resolves the right-clicked row (by index, the same way
// rowClicked does) and hands it to the owner's menu builder with the
// click position. A right-click selects nothing — the menu acts on the
// row it was opened on.
func (t *Table) rowMenu(i int, at image.Point) {
	if i < 0 || i >= len(t.rows) || t.onMenu == nil {
		return
	}
	t.onMenu(t.rows[i], at)
}

// updateIcon refreshes one row's cell icons and their palette role. It runs
// alongside updateBg: selection moves the icon tint to the accent just as it
// moves the row background.
func (t *Table) updateIcon(i int) {
	if i < 0 || i >= len(t.rows) {
		return
	}
	row := t.rows[i]
	tint := t.iconTint(i, row)
	for j, c := range t.cells[i] {
		if c != nil {
			c.box.Set(iconOf(row, j), tint)
		}
	}
}

// updateBg paints the row background (selection accent wins over the
// row's own tint), skipping rows whose background is already painted.
func (t *Table) updateBg(i int) {
	want := t.rowTint(i)
	if i < len(t.applied) && want == t.applied[i] {
		return
	}
	if i < len(t.applied) {
		t.applied[i] = want
	}
	if ctr := t.rowWidget(i); ctr != nil {
		ctr.SetBackgroundImage(ns(solid(want)))
	}
}

// rowTint is the background the row should show right now.
func (t *Table) rowTint(i int) color.RGBA {
	if i == t.selIdx {
		return withA(t.env.Pal.Accent, selectionAlpha)
	}
	if i < 0 || i >= len(t.rows) {
		return color.RGBA{}
	}
	return t.rows[i].Tint
}

// scrollBy moves the viewport by notches wheel-notches of rows. The wheel
// convention is explicit: a positive notch is wheel up (ebiten's delta for
// a wheel rolled towards the user), which scrolls towards the TOP of the
// list — ScrollTop decreases towards 0 and earlier rows are revealed; a
// negative notch scrolls towards the end of the list (ScrollTop grows
// towards 1). ScrollTop is the 0..1 fraction of the content overflow, so
// the step is wheelRows rows of that overflow.
func (t *Table) scrollBy(notches float64) {
	over := t.scrollOverflow()
	if over <= 0 {
		// The rows fit the viewport: nothing to scroll and no stale
		// offset — ebitenui would otherwise bottom-anchor the list.
		t.scroll.ScrollTop = 0
		return
	}
	top := t.scroll.ScrollTop - notches*wheelRows*t.rowH/over
	t.scroll.ScrollTop = math.Max(0, math.Min(1, top))
}

// scrollOverflow is the scrollable distance in pixels: content height
// minus viewport height. It is <= 0 exactly when the rows fit the
// viewport, in which case the list is top-anchored (ScrollTop 0) — the
// content height comes from the body's preferred size, the same value
// renderContent positions with, so it stays correct between SetRows and
// the next layout pass.
func (t *Table) scrollOverflow() float64 {
	_, h := t.body.PreferredSize()
	return float64(h) - float64(t.scroll.ViewRect().Dy())
}

// clampScroll pins ScrollTop to the current overflow: 0 while the rows
// fit the viewport (a leftover offset would bottom-anchor the list), else
// within the 0..1 fraction. Rows are re-measured on every SetRows and the
// viewport can grow on window resize, so both paths call this.
func (t *Table) clampScroll() {
	if over := t.scrollOverflow(); over <= 0 {
		t.scroll.ScrollTop = 0
		return
	}
	t.scroll.ScrollTop = math.Max(0, math.Min(1, t.scroll.ScrollTop))
}

// applyWidths measures the natural width of every column (header plus
// the widest cell of the current rows), fits them into the available
// width and pushes the result into the header buttons and row cells.
func (t *Table) applyWidths() {
	avail := float64(t.root.GetWidget().Rect.Dx())
	natural := make([]ColWidth, len(t.cols))
	prio := make([]int, len(t.cols))
	for i, c := range t.cols {
		w := t.headMeasure(c.Title+t.indicator(i)) + 2*cellPad
		if c.Width > w {
			w = c.Width
		}
		for _, row := range t.rows {
			m := t.measure(cellOf(row, i)) + 2*cellPad
			if iconOf(row, i) != "" && !c.AlignRight {
				m += IconInline + IconGap
			}
			if m > w {
				w = m
			}
		}
		min := t.headMeasure(c.Title+t.indicator(i)) + 2*cellPad
		if c.MinWidth > min {
			min = c.MinWidth
		}
		natural[i] = ColWidth{Label: c.Title, Width: w, Min: min}
		prio[i] = c.Priority
	}
	fitted := natural
	if avail > 0 {
		fitted = fitColumns(natural, prio, growColumn(prio), avail)
	}
	t.fittedFor = avail
	t.widths = make([]float64, len(fitted))
	for i, c := range fitted {
		t.widths[i] = c.Width
	}
	t.layoutCells()
}

// layoutCells applies the fitted widths: cumulative rounding gives every
// column a fixed anchor (no jitter), the header buttons tile the strip
// exactly, and every cell gets its width plus an ellipsized label.
func (t *Table) layoutCells() {
	prev := 0
	acc := 0.0
	for i, c := range t.cols {
		acc += t.widths[i]
		w := int(math.Round(acc)) - prev
		prev += w

		b := t.hdrBtn[i]
		b.GetWidget().MinWidth = w
		b.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true, MaxWidth: w}
		b.SetText(c.Title + t.indicator(i))
		if t.sortKey != "" && t.sortKey == c.Key {
			b.SetImage(t.hdrOn)
		} else {
			b.SetImage(t.hdrIdle)
		}

		// Ellipsize against the float width: the int-rounded cell is up
		// to half a pixel narrower, and rounding it down here would
		// ellipsize text that measurably fits ("25%" -> "2…"). An icon
		// cell reserves its icon box out of the label's share.
		for r := range t.rows {
			inner := t.widths[i] - 2*cellPad
			if t.cells[r][i] != nil && iconOf(t.rows[r], i) != "" {
				inner -= IconInline + IconGap
			}
			if inner < 0 {
				inner = 0
			}
			tx := t.texts[r][i]
			tx.Label = Ellipsize(cellOf(t.rows[r], i), inner, t.measure)
			root := t.cellRoot(r, i)
			root.MinWidth = w
			root.LayoutData = widget.RowLayoutData{Stretch: true, MaxWidth: w}
		}
	}
	t.header.RequestRelayout()
	for _, ctr := range t.rowCts {
		ctr.RequestRelayout()
	}
	t.body.RequestRelayout()
}

// indicator is the active sort column's arrow suffix. Turret Road ships
// no U+25B2/U+25BC triangles, so the arrows are the ↑/↓ it does have.
func (t *Table) indicator(i int) string {
	if t.sortKey == "" || t.cols[i].Key != t.sortKey {
		return ""
	}
	if t.sortAsc {
		return " ↑"
	}
	return " ↓"
}

// cellOf returns row cell i ("" when the row is short).
func cellOf(row TableRow, i int) string {
	if i < 0 || i >= len(row.Cells) {
		return ""
	}
	return row.Cells[i]
}

// sameRow reports whether two rows are the same logical row for the
// double-click window: Data first, cell contents as the nil-Data fallback.
func sameRow(a, b TableRow) bool {
	if a.Data != nil || b.Data != nil {
		return sameData(a.Data, b.Data)
	}
	if len(a.Cells) != len(b.Cells) {
		return false
	}
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			return false
		}
	}
	return true
}

// sameData compares row payloads safely for any dynamic type.
func sameData(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return reflect.DeepEqual(a, b)
}

// iconTint picks a row icon's palette role: the selected row wears the
// accent, a row with its own tint (own ship → ColInfo, hostiles → ColDanger)
// keeps it, everything else is primary text.
func (t *Table) iconTint(i int, row TableRow) color.RGBA {
	if i == t.selIdx {
		return t.env.Pal.Accent
	}
	if row.IconTint.A != 0 {
		return row.IconTint
	}
	return t.env.Pal.Text
}

// withA returns c with a replacement alpha channel.
func withA(c color.RGBA, a uint8) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// growColumn is the column that receives surplus width: the lowest
// priority one (the most important column; first wins ties).
func growColumn(prio []int) int {
	best := 0
	for i := 1; i < len(prio); i++ {
		if prio[i] < prio[best] {
			best = i
		}
	}
	return best
}

// fitColumns fits natural column widths into avail pixels. When there is
// surplus space it all goes to the grow column; when space is short,
// columns shrink toward their Min by descending priority (ties: lower
// index first) and any residual overflow is left alone — the leftover
// columns ellipsize/clip rather than dropping the header.
// Upgrading this means a horizontal scrollbar.
func fitColumns(cols []ColWidth, prio []int, grow int, avail float64) []ColWidth {
	sum := 0.0
	for _, c := range cols {
		sum += c.Width
	}
	if sum <= avail {
		if grow >= 0 && grow < len(cols) {
			cols[grow].Width += avail - sum
		}
		return cols
	}
	deficit := sum - avail
	order := make([]int, len(cols))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return prio[order[a]] > prio[order[b]] })
	for _, i := range order {
		take := cols[i].Width - cols[i].Min
		if take <= 0 {
			continue
		}
		if take > deficit {
			take = deficit
		}
		cols[i].Width -= take
		deficit -= take
		if deficit <= 0 {
			return cols
		}
	}
	return cols
}
