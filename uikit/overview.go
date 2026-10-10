package uikit

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// Layout constants for the overview window chrome (derived from the
// legacy ui padding scale).
const (
	titleBarH   = 44 // title line + breathing room (ui.TitleBarH at 21 px)
	contentsPad = 5  // panel margins inside a window
)

// ovRow is the overview's persistent per-entity row payload: the pointer
// identity doubles as the Table's stable selection/activation key while
// info refreshes every sync (distances tick, flags change).
type ovRow struct {
	info sim.Info
}

// overview owns the Overview window: a WindowPanel whose body is a
// Table of the visible grid.
type overview struct {
	win  *widget.Window
	tbl  *Table
	rows map[ecs.Entity]*ovRow // persistent row objects keyed by entity
}

// panelEnv packages the kit's live state as the Env its panels are
// built from (kit.go owns the kit's construction; this shares its
// theme, palette and enqueue seam instead of building a second one).
func (k *Kit) panelEnv() Env {
	return Env{
		Content: k.Content,
		Sim:     k.Sim,
		Theme:   k.theme,
		Pal:     k.palette,
		TS:      k.scale,
		Enqueue: k.enqueue,
		faces:   k.faces,
	}
}

// AddWindow mounts an extra window on the kit's UI so it shares the
// kit's theme, stacking and input pump. kit.go (owned by integration)
// only builds the overview and selected panels; the station windows
// join through this seam. A window built with makeEdgeResizable joins
// the kit's resize-chrome pump here. The returned func closes the
// window again.
func (k *Kit) AddWindow(w *widget.Window) widget.RemoveWindowFunc {
	if ch, ok := chromeByWindow[w]; ok {
		dup := false
		for _, have := range k.chromes {
			if have == ch {
				dup = true
				break
			}
		}
		if !dup {
			k.chromes = append(k.chromes, ch)
		}
	}
	return k.ui.AddWindow(w)
}

// buildOverview constructs the Overview window around its Table.
func (k *Kit) buildOverview() {
	o := &overview{rows: map[ecs.Entity]*ovRow{}}
	k.ov = o

	tbl := NewTable(k.panelEnv(), []Column{
		{Key: "Name", Title: "Name", Priority: 0},
		{Key: "Distance", Title: "Distance", Priority: 1, AlignRight: true},
		{Key: "Kind", Title: "Kind", Priority: 2},
		{Key: "Velocity", Title: "Velocity", Priority: 3, AlignRight: true},
	})
	// Typed sort: string cells would order "1.2 AU" before "42 m".
	tbl.SetSortFunc(sortOverview)
	tbl.SetSort("Distance", true)
	tbl.OnSelect(func(r TableRow) {
		if row, ok := r.Data.(*ovRow); ok {
			k.selected = row.info.Entity
		}
	})
	tbl.OnActivate(func(r TableRow) {
		if row, ok := r.Data.(*ovRow); ok {
			k.enqueue(sim.CmdApproach{Target: row.info.Entity})
		}
	})
	o.tbl = tbl

	// Body: the table fills its grid cell exactly, so the scroll
	// viewport is bounded by the window no matter how tall the rows are.
	contents := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(k.palette.BG), nrgba(k.palette.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	contents.AddChild(tbl.Container())

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(k.palette.BGHard), nrgba(k.palette.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Overview", facePtr(k.faces.base), solid(k.palette.Accent)),
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
		// Resize comes from the winchrome edge grips (all four edges),
		// not widget.WindowOpts.Resizeable() — that one only stretches
		// the bottom/right corner. Attached in NewKit.
		widget.WindowOpts.MinSize(320, 240),
		widget.WindowOpts.Location(image.Rect(8, 8, 388, 428)),
	)
}

// sync refreshes the rows from the sim: visible grid entities only, with
// hostile/player tints, then lets the table sort and diff the widgets.
func (o *overview) sync(k *Kit) {
	order := make([]TableRow, 0, 32)
	for _, info := range k.Sim.Grid() {
		if !info.Visible {
			continue
		}
		var tint color.RGBA
		switch {
		case info.Entity == k.Sim.Player():
			tint = withA(k.palette.Player, 60)
		case info.Hostile:
			tint = withA(k.palette.Danger, 60)
		}
		order = append(order, TableRow{
			Cells: []string{
				info.Name,
				formatDistance(info.Distance),
				kindLabel(info.Kind),
				formatSpeed(info.Vel.Len()),
			},
			Tint: tint,
			Data: o.rowFor(info),
		})
	}
	// Prune row objects of entities that left the grid (selection
	// identity survives because it lives on the *ovRow, not the index).
	if len(o.rows) != len(order) {
		fresh := make(map[ecs.Entity]*ovRow, len(order))
		for _, r := range order {
			row := r.Data.(*ovRow)
			fresh[row.info.Entity] = row
		}
		o.rows = fresh
	}
	o.tbl.SetRows(order)
	o.tbl.Sync()
}

// rowFor returns the persistent row object for an entity, refreshing its
// info snapshot.
func (o *overview) rowFor(info sim.Info) *ovRow {
	r := o.rows[info.Entity]
	if r == nil {
		r = &ovRow{}
		o.rows[info.Entity] = r
	}
	r.info = info
	return r
}

// selectEntity syncs the row highlight to an externally chosen selection
// (a world click or keyboard cycle). Unknown entities clear the highlight.
func (o *overview) selectEntity(e ecs.Entity) {
	if r, ok := o.rows[e]; ok {
		o.tbl.SelectData(r)
		return
	}
	o.tbl.SelectData(nil)
}

// drawTints is the kit.go Draw seam: the Table paints row tints itself
// now (per-row backgrounds), so there is no post-draw tint pass left.
func (o *overview) drawTints(screen *ebiten.Image, k *Kit) {}

// sortOverview orders overview rows stably by their typed Info values;
// equal keys keep their previous relative order.
func sortOverview(key string, asc bool, rows []TableRow) {
	less := func(a, b TableRow) bool {
		ia, okA := a.Data.(*ovRow)
		ib, okB := b.Data.(*ovRow)
		if !okA || !okB {
			return false
		}
		switch key {
		case "Name":
			return ia.info.Name < ib.info.Name
		case "Distance":
			return ia.info.Distance < ib.info.Distance
		case "Kind":
			return ia.info.Kind < ib.info.Kind
		case "Velocity":
			return ia.info.Vel.Len() < ib.info.Vel.Len()
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

// ---------------------------------------------------------------------------
// Column fitting — FitColumns is the legacy elastic fit the overview
// shipped with (surplus to Name, shrink order Name-last); Table uses the
// generic fitColumns with per-column priorities. Pure and unit-tested.
// ---------------------------------------------------------------------------

// ColWidth is one column's measurement input and fitted output for
// FitColumns: Width is the natural (content + padding) width on input
// and the fitted width on output; Min is the shrink floor (header +
// padding).
type ColWidth struct {
	Label string
	Width float64
	Min   float64
}

// FitColumns fits natural column widths into avail pixels. When there is
// surplus space it is all given to the Name column; when space is short,
// columns shrink toward their Min, later columns first (Velocity gives
// way before Name), and Name never drops below its Min (its cells
// ellipsize instead). Pure and unit-tested.
func FitColumns(cols []ColWidth, avail float64) []ColWidth {
	prio := make([]int, len(cols))
	for i := range prio {
		prio[i] = i // shrink order: highest index yields first
	}
	return fitColumns(cols, prio, nameColumn(cols), avail)
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

// formatCredits renders an amount of currency as "12,345.00 cr"
// (the currency of Entieve is the credit).
func formatCredits(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	v = math.Round(v*100) / 100
	ip := int64(v)
	frac := int64(math.Round((v - float64(ip)) * 100))
	if frac >= 100 { // carry from rounding, e.g. 0.999 -> 1.00
		ip++
		frac = 0
	}
	s := groupInt(ip) + "." + pad2(frac) + " cr"
	if neg {
		s = "-" + s
	}
	return s
}

// formatVolume renders a volume in m³ with one decimal: "123.4 m³".
func formatVolume(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + " m³"
}

// formatPct renders a fraction 0..1 as "87%".
func formatPct(f float64) string {
	return strconv.FormatInt(int64(math.Round(f*100)), 10) + "%"
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

// pad2 renders a 0..99 integer with a leading zero.
func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
