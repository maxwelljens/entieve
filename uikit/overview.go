package uikit

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Layout constants for the overview window chrome (derived from the legacy
// ui padding scale).
const (
	titleBarH    = 44 // title line + breathing room (ui.TitleBarH at 21 px)
	contentsPad  = 5  // panel margins inside a window
	headerSpace  = 4  // gap between header row and list
	cellPad      = 5  // horizontal padding inside every column
	entryPadV    = 5  // vertical padding inside a list row
	bootstrapMax = 48 // rows kept before the first measured refit
)

// doubleClickWindow is the maximum gap between two row activations that
// still counts as a double click (approach).
const doubleClickWindow = 350 * time.Millisecond

// SortColumn selects the overview sort key.
type SortColumn int

// Overview sort columns, in header order.
const (
	SortName SortColumn = iota
	SortDistance
	SortKind
	SortVelocity
)

// sortHeaders are the column header labels, indexed by SortColumn.
var sortHeaders = [...]string{"Name", "Distance", "Kind", "Velocity"}

// ColWidth is one overview column's measurement input and fitted output for
// FitColumns: Width is the natural (content + padding) width on input and
// the fitted width on output; Min is the shrink floor (header + padding).
type ColWidth struct {
	Label string
	Width float64
	Min   float64
}

// shrinkOrder is the fixed column shrink priority when the natural widths
// overflow the available space: Velocity gives way first, Name last.
var shrinkOrder = [...]SortColumn{SortVelocity, SortKind, SortDistance, SortName}

// FitColumns fits natural column widths into avail pixels. When there is
// surplus space it is all given to the Name column; when space is short,
// columns shrink toward their Min in shrinkOrder (Velocity first), and
// Name shrinks last — Name never drops below its Min (its cells ellipsize
// instead). Pure and unit-tested.
func FitColumns(cols []ColWidth, avail float64) []ColWidth {
	sum := 0.0
	for _, c := range cols {
		sum += c.Width
	}
	if sum <= avail {
		// Surplus goes to Name (fallback: first column).
		i := nameColumn(cols)
		cols[i].Width += avail - sum
		return cols
	}
	deficit := sum - avail
	for _, sc := range shrinkOrder {
		i := int(sc)
		if i >= len(cols) {
			continue
		}
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
	// Pathological (every column at Min, still over): leave the overflow —
	// Name cells ellipsize and numeric cells clip rather than dropping the
	// header. Upgrading this means a horizontal scrollbar.
	return cols
}

// nameColumn returns the index of the column named "Name".
func nameColumn(cols []ColWidth) int {
	for i, c := range cols {
		if c.Label == "Name" {
			return i
		}
	}
	return 0
}

// Ellipsize shortens s with a trailing "…" so the result measures at most
// w, using measure (which must be monotonic in string length). An empty
// string is returned when even the ellipsis does not fit.
func Ellipsize(s string, w float64, measure func(string) float64) string {
	if measure(s) <= w {
		return s
	}
	const dots = "…"
	if measure(dots) > w {
		return ""
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(string(runes[:mid])+dots) <= w {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return dots
	}
	return string(runes[:lo]) + dots
}

// Row is one overview entry: the sim read model plus the composed label
// currently shown in the list. Rows are persistent objects keyed by entity
// so list entries keep a stable identity across syncs.
type Row struct {
	info  sim.Info
	label string
	dirty bool // label changed since it was pushed to the list
}

// overview owns the resizable Overview window: header sort buttons, the
// windowed entity list and the elastic column fit.
type overview struct {
	win    *widget.Window
	list   *widget.List
	header *widget.Container
	btns   [len(sortHeaders)]*widget.Button

	palette   Palette
	entryFace text.Face
	measure   func(string) float64
	spaceW    float64

	sortCol SortColumn
	sortAsc bool

	rows      map[ecs.Entity]*Row // persistent row objects
	order     []*Row              // every visible row, sorted
	page      []*Row              // rows currently loaded into the list
	first     int                 // index of the first page row
	measuredN int                 // row count the last refit measured

	cols     [4]ColWidth
	rowH     float64 // one list row's height in px (measured)
	visibleN int     // rows that fit the viewport
	refitDue bool
	frame    int

	// lastClick/lastClickAt track the double-click window for row
	// activation (List has no double-click event).
	lastClick   *Row
	lastClickAt time.Time
}

// buildOverview constructs the Overview window and registers its handlers.
func (k *Kit) buildOverview() {
	p := k.palette
	o := &overview{
		palette:   p,
		entryFace: k.faces.small,
		sortCol:   SortDistance,
		sortAsc:   true,
		rows:      map[ecs.Entity]*Row{},
		refitDue:  true,
	}
	o.measure = func(s string) float64 {
		w, _ := text.Measure(s, o.entryFace, 0)
		return w
	}
	o.spaceW = o.measure(" ")
	k.ov = o

	// Header: one sort button per column; refit pins each button's width
	// to the fitted column so headers stay aligned with row cells.
	o.header = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(eimage.NewNineSliceColor(solid(p.HeadBg))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)
	for i, label := range sortHeaders {
		// Text alignment mirrors the row cells: Name/Kind flush left,
		// Distance/Velocity flush right — with the same 5 px column pad,
		// so header text and cell text share x positions exactly.
		align := widget.TextPositionStart
		if SortColumn(i) == SortDistance || SortColumn(i) == SortVelocity {
			align = widget.TextPositionEnd
		}
		btn := widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.TextPosition(align, widget.TextPositionCenter),
			widget.ButtonOpts.TextPadding(&widget.Insets{
				Left: cellPad, Right: cellPad, Top: 4, Bottom: 4,
			}),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, 40),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
				k.sortClicked(SortColumn(i))
			}),
		)
		o.btns[i] = btn
		o.header.AddChild(btn)
	}

	// Rows: ebitenui's List with its internal scrollbar hidden. Scrolling
	// is wheel-driven through the list's own ScrolledEvent and pages the
	// entries (EVE-style), which keeps every rendered row's geometry
	// exactly known for the hostile/player tint pass.
	o.list = widget.NewList(
		widget.ListOpts.ContainerOpts(widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		)),
		widget.ListOpts.EntryLabelFunc(func(e any) string { return e.(*Row).label }),
		widget.ListOpts.EntryFontFace(facePtr(k.faces.small)),
		// No horizontal inset: composeRow owns column padding, so row
		// cells and header text share the same x positions.
		widget.ListOpts.EntryTextPadding(&widget.Insets{
			Top: entryPadV, Bottom: entryPadV,
		}),
		widget.ListOpts.EntryTextPosition(widget.TextPositionStart, widget.TextPositionCenter),
		widget.ListOpts.AllowReselect(),
		widget.ListOpts.HideVerticalSlider(),
		widget.ListOpts.HideHorizontalSlider(),
		widget.ListOpts.EntrySelectedHandler(k.entrySelected),
	)
	// Wheel pages through the full sorted order (the list itself never
	// scrolls internally: its content always fits the viewport).
	o.list.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			o.scrollBy(int(ev.Y) * 3)
		}
	})

	contents := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(headerSpace),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	contents.AddChild(o.header)
	contents.AddChild(o.list)

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Overview", facePtr(k.faces.base), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	))

	o.win = widget.NewWindow(
		widget.WindowOpts.Contents(contents),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.Resizeable(),
		widget.WindowOpts.MinSize(320, 240),
		widget.WindowOpts.Location(image.Rect(8, 8, 8+540, 8+600)),
		// Resizing the window reflows the elastic columns.
		widget.WindowOpts.ResizeHandler(func(*widget.WindowChangedEventArgs) {
			o.refitColumns()
		}),
	)
}

// sync refreshes rows, sorting, columns and list entries from the sim.
func (o *overview) sync(k *Kit) {
	o.frame++

	// 1 — visible rows, in deterministic grid order.
	order := make([]*Row, 0, 32)
	for _, info := range k.Sim.Grid() {
		if !info.Visible {
			continue
		}
		r := o.rows[info.Entity]
		if r == nil {
			r = &Row{}
			o.rows[info.Entity] = r
		}
		r.info = info
		order = append(order, r)
	}
	if len(o.rows) != len(order) {
		fresh := make(map[ecs.Entity]*Row, len(order))
		for _, r := range order {
			fresh[r.info.Entity] = r
		}
		o.rows = fresh
	}

	// 2 — sort (stable; presentation only, sim.Grid order is untouched).
	sortRows(order, o.sortCol, o.sortAsc)
	o.order = order

	// 3 — refit columns when the structure changed, a resize/sort asked
	// for it, or periodically (long names appearing later).
	if o.refitDue || len(order) != o.measuredN || o.frame%90 == 0 {
		o.refitDue = false
		o.refitColumns()
	}

	// 4 — page the rows that fit the viewport.
	n := o.pageSize(len(order))
	if o.first > len(order)-n {
		o.first = max(0, len(order)-n)
	}
	end := min(o.first+n, len(order))
	if o.first > end {
		o.first = end
	}
	page := order[o.first:end]
	o.visibleN = n

	// 5 — compose labels for the visible page.
	for _, r := range page {
		if lbl := o.composeRow(r); lbl != r.label {
			r.label = lbl
			r.dirty = true
		}
	}

	// 6 — push the page into the list (rebuild on membership change,
	// per-row update otherwise).
	rebuilt := false
	if !equalPage(o.list.Entries(), page) {
		o.list.SetEntries(pageAny(page))
		rebuilt = true
		// SetEntries drops the selection silently; re-apply it.
		if r := o.rowFor(k.selected); r != nil && containsRow(page, r) {
			o.list.SetSelectedEntry(r)
		}
	}
	for _, r := range page {
		if rebuilt || !r.dirty {
			r.dirty = false
			continue
		}
		o.list.UpdateEntry(r)
		r.dirty = false
	}
	o.page = page

	// 7 — measure row height exactly: the list's preferred height
	// contains a page-independent constant (slider chrome etc.), so a
	// single pref/n would overestimate and the tints would drift. Two
	// samples cancel the constant. Runs once (rowH stays fixed: rows are
	// single-line and uniform).
	if o.rowH <= 0 && len(page) > 0 {
		o.rowH = o.measureRowH()
	}
}

// measureRowH returns the exact height of one list row by diffing the
// list's preferred height for n and n−1 entries. Falls back to the font
// metrics formula when fewer than two rows are available.
func (o *overview) measureRowH() float64 {
	entries := o.list.Entries()
	if len(entries) >= 2 {
		_, hFull := o.list.PreferredSize()
		o.list.SetEntries(entries[:len(entries)-1])
		_, hLess := o.list.PreferredSize()
		o.list.SetEntries(entries)
		if d := hFull - hLess; d > 0 {
			return float64(d)
		}
	}
	// Fallback: text line box plus the entry text padding.
	m := o.entryFace.Metrics()
	return math.Ceil(m.HAscent+m.HDescent) + 2*entryPadV
}

// rowFor returns the persistent row object for an entity (nil if none).
func (o *overview) rowFor(e ecs.Entity) *Row {
	if e.IsZero() {
		return nil
	}
	return o.rows[e]
}

// pageSize returns how many rows fit the list viewport.
func (o *overview) pageSize(orderLen int) int {
	if orderLen == 0 {
		return 0
	}
	if o.rowH <= 0 { // first sync: bootstrap with a capped page
		return min(orderLen, bootstrapMax)
	}
	win := o.win.GetContainer().GetWidget().Rect
	_, headerH := o.header.PreferredSize()
	viewH := float64(win.Dy()) - titleBarH - float64(headerH) - 2*contentsPad - headerSpace
	n := int(viewH / o.rowH)
	if n < 1 {
		n = 1
	}
	return n
}

// listWidth returns the pixel width available to the columns: the window
// width minus the contents padding (read from the window so a resize is
// reflected before the next relayout).
func (o *overview) listWidth() float64 {
	win := o.win.GetContainer().GetWidget().Rect
	return float64(win.Dx()) - 2*contentsPad
}

// refitColumns measures header and cell text of all visible rows, applies
// FitColumns to the window width and pushes the result into the header
// buttons.
func (o *overview) refitColumns() {
	avail := o.listWidth()
	if avail <= 0 {
		return
	}
	var natural [len(sortHeaders)]float64
	for i, h := range sortHeaders {
		label := h
		if SortColumn(i) == o.sortCol {
			label += o.indicator(SortColumn(i))
		}
		natural[i] = o.measure(label)
	}
	for _, r := range o.order {
		cells := [...]string{
			r.info.Name,
			formatDistance(r.info.Distance),
			kindLabel(r.info.Kind),
			formatSpeed(r.info.Vel.Len()),
		}
		for i, c := range cells {
			if w := o.measure(c); w > natural[i] {
				natural[i] = w
			}
		}
	}
	var cols [len(sortHeaders)]ColWidth
	for i, h := range sortHeaders {
		minW := o.measure(h) + 2*cellPad
		cols[i] = ColWidth{Label: h, Width: natural[i] + 2*cellPad, Min: minW}
	}
	copy(o.cols[:], FitColumns(cols[:], avail))
	o.measuredN = len(o.order)
	o.applyHeader()
}

// applyHeader pins every sort button to its fitted column width and
// refreshes the ▲/▼ indicator label. Widths are rounded cumulatively so
// the buttons tile the row exactly — independent ceils would overshoot
// the panel edge by up to len(cols) pixels.
func (o *overview) applyHeader() {
	prev := 0
	acc := 0.0
	for i, btn := range o.btns {
		acc += o.cols[i].Width
		w := int(math.Round(acc)) - prev
		prev += w // running total: round(acc)
		btn.GetWidget().MinWidth = w
		btn.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true, MaxWidth: w}
		btn.Text().Label = sortHeaders[i] + o.indicator(SortColumn(i))
	}
	o.header.RequestRelayout()
}

// indicator is the active sort column's arrow suffix.
func (o *overview) indicator(c SortColumn) string {
	if c != o.sortCol {
		return ""
	}
	if o.sortAsc {
		return " ▲"
	}
	return " ▼"
}

// sortClicked toggles the sort column/direction and re-measures the
// headers (the indicator changes their width); the row order itself is
// applied by the next Sync so the drawn list and page never disagree.
func (k *Kit) sortClicked(c SortColumn) {
	o := k.ov
	if o.sortCol == c {
		o.sortAsc = !o.sortAsc
	} else {
		o.sortCol = c
		o.sortAsc = true
	}
	o.refitColumns()
}

// scrollBy moves the page window by delta rows (wheel up scrolls toward
// the top). Applied by the next Sync.
func (o *overview) scrollBy(delta int) {
	o.first -= delta
	if o.first < 0 {
		o.first = 0
	}
	if maxFirst := len(o.order) - o.visibleN; o.first > maxFirst {
		o.first = maxFirst
	}
	if o.first < 0 {
		o.first = 0
	}
}

// entrySelected handles row activation: click selects, a second click on
// the same row within doubleClickWindow approaches it.
func (k *Kit) entrySelected(args *widget.ListEntrySelectedEventArgs) {
	r, ok := args.Entry.(*Row)
	if !ok || r == nil {
		return
	}
	now := time.Now()
	if prev, ok := args.PreviousEntry.(*Row); ok && prev == r &&
		k.ov.lastClick == r && now.Sub(k.ov.lastClickAt) < doubleClickWindow {
		k.enqueue(sim.CmdApproach{Target: r.info.Entity})
	}
	k.ov.lastClick, k.ov.lastClickAt = r, now
	k.selected = r.info.Entity
}

// composeRow renders one row as a single left-aligned label whose cells
// land inside the fitted column boxes: left-aligned Name/Kind, right-
// aligned Distance/Velocity, each column re-anchored by measuring the
// running prefix.
func (o *overview) composeRow(r *Row) string {
	cells := [...]string{
		r.info.Name,
		formatDistance(r.info.Distance),
		kindLabel(r.info.Kind),
		formatSpeed(r.info.Vel.Len()),
	}
	right := [...]bool{false, true, false, true}
	var b strings.Builder
	x := 0.0
	for i, c := range cells {
		avail := o.cols[i].Width - 2*cellPad
		if avail < 0 {
			avail = 0
		}
		if o.measure(c) > avail {
			c = Ellipsize(c, avail, o.measure)
		}
		target := x + cellPad
		if right[i] {
			target = x + o.cols[i].Width - cellPad - o.measure(c)
		}
		if lead := target - o.measure(b.String()); lead > 0 {
			b.WriteString(strings.Repeat(" ", int(math.Ceil(lead/o.spaceW))))
		}
		b.WriteString(c)
		x += o.cols[i].Width
	}
	return b.String()
}

// drawTints paints the per-row background tints over the list after the
// whole UI has drawn: player row in ColPlayer, hostile rows in ColDanger.
// Selected and hover states come from the theme (drawn under the text).
func (o *overview) drawTints(screen *ebiten.Image, k *Kit) {
	r := o.list.GetWidget().Rect
	if r.Dx() <= 0 || r.Dy() <= 0 || o.rowH <= 0 {
		return
	}
	for i, row := range o.page {
		y0 := float64(r.Min.Y) + float64(i)*o.rowH
		y1 := y0 + o.rowH
		if y1 <= float64(r.Min.Y) {
			continue
		}
		if y0 >= float64(r.Max.Y) {
			break
		}
		y0 = max(y0, float64(r.Min.Y))
		y1 = min(y1, float64(r.Max.Y))
		var c color.NRGBA
		switch {
		case row.info.Entity == k.selected:
			continue // theme already draws the accent background
		case row.info.Entity == k.Sim.Player():
			c = nrgba(o.palette.Player, 60)
		case row.info.Hostile:
			c = nrgba(o.palette.Danger, 60)
		default:
			continue
		}
		vector.DrawFilledRect(screen, float32(r.Min.X), float32(y0),
			float32(r.Dx()), float32(y1-y0), c, false)
	}
}

// sortRows sorts rows stably by the given column: ascending picks the
// key's natural order, descending reverses the comparison while equal keys
// keep their previous relative order.
func sortRows(rows []*Row, col SortColumn, asc bool) {
	less := func(a, b *Row) bool {
		switch col {
		case SortName:
			return a.info.Name < b.info.Name
		case SortDistance:
			return a.info.Distance < b.info.Distance
		case SortKind:
			return a.info.Kind < b.info.Kind
		case SortVelocity:
			return a.info.Vel.Len() < b.info.Vel.Len()
		default:
			return false
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if asc {
			return less(rows[i], rows[j])
		}
		return less(rows[j], rows[i])
	})
}

// equalPage reports whether the list already holds exactly this page.
func equalPage(entries []any, page []*Row) bool {
	if len(entries) != len(page) {
		return false
	}
	for i, e := range entries {
		r, ok := e.(*Row)
		if !ok || r != page[i] {
			return false
		}
	}
	return true
}

// pageAny converts a row page to the list's []any entry form.
func pageAny(page []*Row) []any {
	out := make([]any, len(page))
	for i, r := range page {
		out[i] = r
	}
	return out
}

// containsRow reports whether r is in page.
func containsRow(page []*Row, r *Row) bool {
	for _, p := range page {
		if p == r {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Value formatting — local copies of the legacy ui formatters (uikit must
// not import ui outside tests; TestFormattersMatchLegacy pins behavior).
// ---------------------------------------------------------------------------

// formatDistance renders a distance in meters EVE-style: "42 m" /
// "1,250 m" (under 2 km), "3.4 km" (under 0.1 AU), "1.2 AU" beyond.
func formatDistance(m float64) string {
	if m < 0 {
		m = 0
	}
	switch {
	case m < 2e3:
		return groupInt(int64(math.Round(m))) + " m"
	case m < 1.496e10: // 0.1 AU: EVE switches to AU here
		return groupFloat(m/1000, 1) + " km"
	default:
		return groupFloat(m/1.496e11, 1) + " AU"
	}
}

// formatSpeed renders a speed in whole m/s with separators.
func formatSpeed(v float64) string {
	if v < 0 {
		v = 0
	}
	return groupInt(int64(math.Round(v))) + " m/s"
}

// kindLabel title-cases an Info.Kind for display ("asteroid" → "Asteroid").
func kindLabel(kind string) string {
	r := []rune(kind)
	if len(r) == 0 {
		return ""
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// groupInt formats n with thousands separators: 12345 → "12,345".
func groupInt(n int64) string {
	return addCommas(strconv.FormatInt(n, 10))
}

// groupFloat formats v with prec decimals and separators.
func groupFloat(v float64, prec int) string {
	return addCommas(strconv.FormatFloat(v, 'f', prec, 64))
}

// addCommas inserts thousands separators into a decimal string's integer
// part (copied from ui/format.go).
func addCommas(s string) string {
	head, tail, found := strings.Cut(s, ".")
	neg := strings.HasPrefix(head, "-")
	if neg {
		head = head[1:]
	}
	var b strings.Builder
	b.Grow(len(head) + len(head)/3)
	for i, c := range head {
		if i > 0 && (len(head)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	if found {
		out += "." + tail
	}
	return out
}
