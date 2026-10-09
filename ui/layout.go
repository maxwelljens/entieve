package ui

// Rect is an axis-aligned UI rectangle in logical pixels.
type Rect struct{ X, Y, W, H float64 }

// Contains reports whether the point lies inside the rect (top-left inclusive).
func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Right returns the right edge (exclusive).
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns the bottom edge (exclusive).
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Inset shrinks the rect by d on every side.
func (r Rect) Inset(d float64) Rect {
	return Rect{X: r.X + d, Y: r.Y + d, W: r.W - 2*d, H: r.H - 2*d}
}

// Overlaps reports whether two rects share any area.
func (r Rect) Overlaps(o Rect) bool {
	return r.X < o.Right() && o.X < r.Right() && r.Y < o.Bottom() && o.Y < r.Bottom()
}

// Regions holds the rects of every fixed window at one screen size.
// Base panels never overlap; overlays are centered/toggled panels.
type Regions struct {
	Selected  Rect // top-left: selected item + actions
	Overview  Rect // top-right: sortable grid table
	Targets   Rect // top-center: lock/target boxes
	HUD       Rect // bottom-center: shield/armor/structure, cap, speed
	Modules   Rect // bottom-right: high/mid/low module rack
	Log       Rect // bottom-left: combat/action log
	Inventory Rect // overlay: cargo + ore hold
	Fitting   Rect // overlay: slots + derived stats
	Station   Rect // overlay: station services (docked)
	Starmap   Rect // overlay: universe graph
	Help      Rect // overlay: key map
}

// Fixed panel geometry constants.
const (
	margin   = 8.0
	selH     = 136.0
	targetsH = 84.0
	logH     = 160.0
	modH     = 150.0
	hudH     = 110.0
	minSideW = 200.0
	maxSideW = 320.0
)

// ComputeRegions lays out all windows for a w×h screen. The base HUD panels
// fit inside any screen ≥ 800×600 and never overlap.
func ComputeRegions(w, h float64) Regions {
	sideW := clamp(0.25*w, minSideW, maxSideW)
	ovH := clamp(0.42*h, 180, 320)

	midX := margin + sideW + 8
	midW := w - 2*midX
	if midW < 80 {
		midW = 80
	}

	return Regions{
		Selected:  Rect{margin, margin, sideW, selH},
		Overview:  Rect{w - margin - sideW, margin, sideW, ovH},
		Targets:   Rect{midX, margin, midW, targetsH},
		HUD:       Rect{midX, h - margin - hudH, midW, hudH},
		Modules:   Rect{w - margin - sideW, h - margin - modH, sideW, modH},
		Log:       Rect{margin, h - margin - logH, sideW, logH},
		Inventory: Rect{margin, margin + selH + 8, sideW + 60, 260},
		Fitting:   Rect{w - margin - (sideW + 80), margin + selH + 8, sideW + 80, 300},
		Station:   Rect{40, 56, w - 80, h - 140},
		Starmap:   Rect{20, 20, w - 40, h - 40},
		Help:      Rect{w/2 - 270, 56, 540, h - 112},
	}
}

// clamp bounds v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------------
// Sub-layout helpers (pure; shared by Draw and hit-testing)
// ---------------------------------------------------------------------------

// Overview table geometry.
const (
	ovHeaderH = 22.0
	ovRowH    = 20.0
	ovIconW   = 22.0
)

// Overview column widths (distance/velocity scale with the panel so small
// windows keep a usable name column).
func ovDistWidth(r Rect) float64 { return clamp(r.W*0.27, 64, 92) }
func ovVelWidth(r Rect) float64  { return clamp(r.W*0.21, 52, 72) }

// OverviewColumn returns the rect of overview column c (0=icon,1=name,
// 2=distance, 3=velocity) including the header strip.
func OverviewColumn(r Rect, c int) Rect {
	distW, velW := ovDistWidth(r), ovVelWidth(r)
	nameW := r.W - ovIconW - distW - velW
	switch c {
	case 0:
		return Rect{r.X, r.Y, ovIconW, r.H}
	case 1:
		return Rect{r.X + ovIconW, r.Y, nameW, r.H}
	case 2:
		return Rect{r.X + ovIconW + nameW, r.Y, distW, r.H}
	default:
		return Rect{r.X + ovIconW + nameW + distW, r.Y, velW, r.H}
	}
}

// OverviewMaxRows returns how many data rows fit in the overview panel.
func OverviewMaxRows(r Rect) int {
	n := int((r.H - 4 - ovHeaderH) / ovRowH)
	if n < 0 {
		return 0
	}
	return n
}

// OverviewRowRect returns the rect of data row i inside the overview panel.
func OverviewRowRect(r Rect, i int) Rect {
	return Rect{r.X + 1, r.Y + 2 + ovHeaderH + float64(i)*ovRowH, r.W - 2, ovRowH - 1}
}

// OverviewRowAt returns the data row index under y, or -1.
func OverviewRowAt(r Rect, y float64) int {
	i := int((y - (r.Y + 2 + ovHeaderH)) / ovRowH)
	if i < 0 || y >= OverviewRowRect(r, i).Bottom() || y < r.Y+2+ovHeaderH {
		return -1
	}
	if i >= OverviewMaxRows(r) {
		return -1
	}
	return i
}

// OverviewColumnAt returns the column index under x (in header or rows), or -1.
func OverviewColumnAt(r Rect, x float64) int {
	for c := 0; c < 4; c++ {
		if OverviewColumn(r, c).Contains(x, r.Y+1) {
			return c
		}
	}
	return -1
}

// Selected-item action button grid.
const (
	actCols  = 3
	actBtnW  = 96.0
	actBtnH  = 24.0
	actGap   = 6.0
	actStart = 48.0 // offset below the panel top
)

// ActionRects returns up to 3×N button rects inside the selected panel;
// button width adapts to the panel so nothing overflows at 800×600.
func ActionRects(r Rect, n int) []Rect {
	btnW := (r.W - 12 - actGap*(actCols-1)) / actCols
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		col := i % actCols
		row := i / actCols
		out[i] = Rect{
			X: r.X + 6 + float64(col)*(btnW+actGap),
			Y: r.Y + actStart + float64(row)*(actBtnH+actGap),
			W: btnW,
			H: actBtnH,
		}
	}
	return out
}

// Target box geometry (top-center lock list).
const (
	targetBoxW = 76.0
	targetBoxH = 76.0
	targetGap  = 6.0
)

// TargetBoxRects returns up to 4 centered target boxes in the Targets region.
func TargetBoxRects(r Rect, n int) []Rect {
	if n > 4 {
		n = 4
	}
	if n <= 0 {
		return nil
	}
	total := float64(n)*targetBoxW + float64(n-1)*targetGap
	x := r.X + (r.W-total)/2
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		out[i] = Rect{x, r.Y + 4, targetBoxW, targetBoxH}
		x += targetBoxW + targetGap
	}
	return out
}

// Module rack geometry: rows high/mid/low, cells left to right.
const (
	modCell   = 30.0
	modGap    = 4.0
	modLabelH = 12.0
	modRowH   = 46.0
	modLabelW = 34.0
	modPad    = 6.0
	modRows   = 3
)

// ModuleRowLabelRect returns the row label rect ("High"/"Mid"/"Low").
func ModuleRowLabelRect(r Rect, row int) Rect {
	return Rect{r.X + 2, r.Y + modPad + float64(row)*modRowH, modLabelW, modLabelH}
}

// ModuleCellRect returns the cell rect for slot i of the given row.
func ModuleCellRect(r Rect, row, i int) Rect {
	return Rect{
		X: r.X + modLabelW + 4 + float64(i)*(modCell+modGap),
		Y: r.Y + modPad + float64(row)*modRowH + modLabelH,
		W: modCell,
		H: modCell,
	}
}

// ModuleMaxCells returns how many cells fit in one rack row.
func ModuleMaxCells(r Rect) int {
	n := int((r.W - modLabelW - 4 + modGap) / (modCell + modGap))
	if n < 1 {
		n = 1
	}
	return n
}

// ModuleCellAt maps a point to (row, cell) or (-1, -1).
func ModuleCellAt(r Rect, x, y float64) (int, int) {
	for row := 0; row < modRows; row++ {
		if y < r.Y+modPad+float64(row)*modRowH || y >= r.Y+modPad+float64(row+1)*modRowH {
			continue
		}
		for i := 0; i < 16; i++ {
			if ModuleCellRect(r, row, i).Contains(x, y) {
				return row, i
			}
		}
	}
	return -1, -1
}

// Tab strip geometry (station services).
const (
	tabH     = 24.0
	tabStrip = 30.0
)

// TabRects lays n tabs across the top of a panel.
func TabRects(r Rect, n int) []Rect {
	tw := (r.W - 4) / float64(n)
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		out[i] = Rect{r.X + 2 + float64(i)*tw, r.Y + 4, tw - 2, tabH}
	}
	return out
}

// TabAt returns the tab index under the point, or -1.
func TabAt(r Rect, n int, x, y float64) int {
	for i, tr := range TabRects(r, n) {
		if tr.Contains(x, y) {
			return i
		}
	}
	return -1
}

// StationBody returns the content area below the tab strip.
func StationBody(r Rect) Rect {
	return Rect{r.X + 8, r.Y + tabStrip + 6, r.W - 16, r.H - tabStrip - 14}
}

// Fitting window slot rows.
const (
	fitSlotRowH  = 40.0
	fitLabelW    = 34.0
	fitCellW     = 34.0
	fitCellH     = 30.0
	fitCellGap   = 4.0
	fitFirstRowY = 26.0
)

// FittingSlotRow returns the slot-row rect for the given class row.
func FittingSlotRow(r Rect, row int) Rect {
	return Rect{r.X + 6, r.Y + fitFirstRowY + float64(row)*fitSlotRowH, r.W - 12, fitSlotRowH - 6}
}

// FittingCellRect returns slot cell i inside a fitting slot row.
func FittingCellRect(row Rect, i int) Rect {
	return Rect{
		X: row.X + fitLabelW + float64(i)*(fitCellW+fitCellGap),
		Y: row.Y + 2,
		W: fitCellW,
		H: fitCellH,
	}
}

// FittingUnfitRect returns the small [x] button over a filled cell.
func FittingUnfitRect(cell Rect) Rect {
	return Rect{cell.Right() - 13, cell.Y + 1, 12, 12}
}

// FittingMaxCells returns how many fitting cells fit in one slot row.
func FittingMaxCells(r Rect) int {
	rowW := r.W - 12
	n := int((rowW - fitLabelW) / (fitCellW + fitCellGap))
	if n < 1 {
		n = 1
	}
	return n
}

// Station table geometry (market / refine / fitting lists).
const (
	stHeadH = 22.0
	stRowH  = 26.0
)

// StationHeadRect returns the column-header strip of a station table body.
func StationHeadRect(body Rect) Rect {
	return Rect{body.X, body.Y, body.W, stHeadH}
}

// StationRowRect returns data row i inside a station table body.
func StationRowRect(body Rect, i int) Rect {
	return Rect{body.X, body.Y + stHeadH + float64(i)*stRowH, body.W, stRowH - 2}
}

// StationMaxRows returns how many rows fit in a station table body.
func StationMaxRows(body Rect) int {
	n := int((body.H - stHeadH) / stRowH)
	if n < 0 {
		return 0
	}
	return n
}

// StationRowAt returns the data row index under y, or -1.
func StationRowAt(body Rect, y float64) int {
	i := int((y - (body.Y + stHeadH)) / stRowH)
	if i < 0 || y < body.Y+stHeadH {
		return -1
	}
	if i >= StationMaxRows(body) {
		return -1
	}
	if !StationRowRect(body, i).Contains(body.X+1, y) {
		return -1
	}
	return i
}

// RightButton returns a button of width w anchored to a row's right edge.
func RightButton(row Rect, w float64) Rect {
	return Rect{row.Right() - w - 2, row.Y + 2, w, row.H - 4}
}
