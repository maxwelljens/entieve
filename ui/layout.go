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
	Scanner   Rect // bottom-left above the log: off-grid sweep (C)
	Inventory Rect // overlay: cargo + ore hold
	Fitting   Rect // overlay: slots + derived stats
	Station   Rect // overlay: station services (docked)
	Starmap   Rect // overlay: universe graph (centered)
	Help      Rect // overlay: key map
}

// ---------------------------------------------------------------------------
// Chrome metrics — every value derives from the typography + padding
// constants in style.go (no hand-tuned pixels).
// ---------------------------------------------------------------------------

// Selected-item action grid.
var (
	actCols  = 3
	actBtnH  = minHit   // 40 px minimum hit area
	actGap   = padSmall // row/column gap
	actStart = titleBarH + pad + lineH(FontLarge) + lineH(FontSmall) +
		padSmall + warpSegH + padSmall // content offset below the title
	// selH is the full selected-panel height: chrome + identity block +
	// warp-range segmented control + the worst-case 3×3 action grid.
	selH = actStart + actRows*(actBtnH+actGap) - actGap + pad
)

// actRows is the worst-case number of action-button rows (9 actions).
const actRows = 3

// warpSegH is the height of the warp-to-within segmented control.
const warpSegH = minHit

// Target box geometry (top-center lock list).
var (
	targetBoxW = FontBase * 4 // portrait + name width
	targetBoxH = targetBoxW + lineH(FontSmall) + pad
	targetGap  = padSmall
	targetsH   = targetBoxH + pad
)

// HUD geometry: three HP lines, a capacitor bar and its caption.
var (
	hudCapH = lineH(FontSmall)
	hudH    = 2*pad + 3*lineH(FontSmall) + hudCapH + lineH(FontTiny)
	// hudRingR / hudRingStep derive the shield/armor/structure arc radii.
	hudRingR    = hudH/2 - pad
	hudRingStep = padLarge / 2
)

// modRows is the rack row count (high/mid/low).
const modRows = 3

// Module rack geometry: rows high/mid/low, cells left to right.
var (
	modCell   = minHit // 40×40 cells (minimum hit area)
	modGap    = padSmall
	modLabelH = lineH(FontTiny)
	modLabelW = FontBase * 2
	modPad    = pad
	modRowH   = modLabelH + modCell + 2*padSmall
	// modH = title bar + padding + three slot rows.
	modH = titleBarH + modPad + float64(modRows)*modRowH + pad
)

// Overview table geometry.
var (
	ovHeaderH = lineH(FontSmall) + padSmall // column header strip
	ovRowH    = lineH(FontSmall) + 2*padSmall
	ovIconW   = FontBase * 1.33 // ~28 px bracket column
	ovScrollW = 3 * padSmall    // reserved scrollbar strip (always reserved
	// so number-column anchors never shift when rows appear/disappear)
)

// Log geometry (bottom-left).
var (
	logLineH = lineH(FontSmall)
	logLines = 5 // visible lines
	logH     = titleBarH + pad + float64(logLines)*logLineH + pad
)

// Scanner window geometry (bottom-left, above the log).
var (
	scanStatusH = lineH(FontTiny)
	scanBarH    = 18.0
	scanLines   = 3 // anonymous contact count lines (ship/asteroid/wreck)
	scanLineH   = lineH(FontSmall)
	scannerH    = titleBarH + pad + scanStatusH + scanBarH + padSmall +
		float64(scanLines)*scanLineH + pad
)

// Inventory overlay geometry: cargo section (6 rows) + ore section (4
// rows) + the docked hint line.
var (
	invSecRowH  = lineH(FontSmall)
	invMeterH   = 6.0                        // capacity meter under a section header
	invSecHeadH = invSecRowH + invMeterH + 2 // header + meter + gap
	invCargoH   = invSecHeadH + 6*invSecRowH // header + 6 rows
	invOreH     = invSecHeadH + 4*invSecRowH // header + 4 rows
	invHintH    = lineH(FontTiny) + pad      // docked hint line
	invW        = 300.0
	invH        = titleBarH + pad + invCargoH + pad + invOreH + pad + invHintH
)

// Fitting overlay geometry: 3 slot rows + derived-stat lines.
var (
	fitSlotRowH  = minHit + pad // slot row pitch (40 px cell + gap)
	fitLabelW    = FontBase * 2 // "High"/"Mid"/"Low" label column
	fitCellW     = minHit       // 40×40 slot cells
	fitCellH     = minHit
	fitCellGap   = padSmall
	fitFirstRowY = titleBarH + pad
	fitStatLines = 6
)

// Station table geometry (market / refine / fitting lists).
var (
	stHeadH = lineH(FontSmall) + padSmall
	stRowH  = minHit + padSmall // rows carry 40 px action buttons
)

// Tab strip geometry (station services).
var (
	tabH     = minHit // tabs are buttons: 40 px hit area
	tabStrip = 2*padSmall + tabH
)

// Fixed panel width bounds for the side columns.
const (
	minSideW = 260.0
	maxSideW = 340.0
)

// ComputeRegions lays out all windows for a w×h screen. The base HUD panels
// fit inside any screen ≥ 1024×700 (the minimum window size) and never
// overlap.
func ComputeRegions(w, h float64) Regions {
	sideW := sideWidth(w)
	ovH := clamp(0.42*h, 180, 340)

	midX := pad + sideW + pad
	midW := w - 2*midX
	if midW < 80 {
		midW = 80
	}

	fit := FittingRect(w, h)
	inv := InventoryRect(w, h)

	return Regions{
		Selected:  Rect{pad, pad, sideW, selH},
		Overview:  Rect{w - pad - sideW, pad, sideW, ovH},
		Targets:   Rect{midX, pad, midW, targetsH},
		HUD:       Rect{midX, h - pad - hudH, midW, hudH},
		Modules:   Rect{w - pad - sideW, h - pad - modH, sideW, modH},
		Log:       Rect{pad, h - pad - logH, sideW, logH},
		Scanner:   Rect{pad, h - pad - logH - pad - scannerH, sideW, scannerH},
		Inventory: inv,
		Fitting:   fit,
		Station:   StationRect(w, h),
		Starmap:   StarmapRect(w, h),
		Help:      HelpRect(w, h),
	}
}

// FittingRect places the fitting overlay center-left: it never covers the
// top-right overview column (explicit UX requirement).
func FittingRect(w, h float64) Rect {
	fw := clamp(0.42*w, 340, 460)
	fh := titleBarH + pad + 3*fitSlotRowH + float64(fitStatLines)*lineH(FontSmall) + pad
	return Rect{X: pad + sideWidth(w) + pad, Y: (h - fh) / 2, W: fw, H: fh}
}

// sideWidth mirrors the side-column width so the fitting window sits just
// right of the left column at any screen size.
func sideWidth(w float64) float64 { return clamp(0.27*w, minSideW, maxSideW) }

// InventoryRect places the inventory overlay center-right: right of the
// center line, left of the overview column.
func InventoryRect(w, h float64) Rect {
	x := w - pad - sideWidth(w) - pad - invW
	return Rect{X: x, Y: (h - invH) / 2, W: invW, H: invH}
}

// StationRect is the docked station-services overlay (near-fullscreen, room
// below for the undock button).
func StationRect(w, h float64) Rect {
	return Rect{X: padLarge, Y: padLarge, W: w - 2*padLarge, H: h - 2*padLarge - minHit - 2*pad}
}

// StarmapRect is the centered starmap overlay.
func StarmapRect(w, h float64) Rect {
	sw := clamp(0.78*w, 560, w-2*pad)
	sh := clamp(0.78*h, 420, h-2*pad)
	return Rect{X: (w - sw) / 2, Y: (h - sh) / 2, W: sw, H: sh}
}

// HelpRect is the centered key-map overlay.
func HelpRect(w, h float64) Rect {
	hw := clamp(680.0, 0, w-2*pad)
	return Rect{X: (w - hw) / 2, Y: padLarge, W: hw, H: h - 2*padLarge}
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
// Overview geometry (title bar + header + scrollable body)
// ---------------------------------------------------------------------------

// OverviewBody returns the scrollable data area of the overview panel
// (below the title bar and column header, excluding the scrollbar strip).
func OverviewBody(r Rect) Rect {
	return Rect{
		X: r.X + 1,
		Y: r.Y + 1 + titleBarH + ovHeaderH,
		W: r.W - 2 - ovScrollW,
		H: r.H - titleBarH - ovHeaderH - 2,
	}
}

// OverviewHeaderRect returns the column-header strip (below the title bar).
func OverviewHeaderRect(r Rect) Rect {
	return Rect{X: r.X + 1, Y: r.Y + 1 + titleBarH, W: r.W - 2, H: ovHeaderH}
}

// OverviewColumn returns the rect of overview column c (0=icon,1=name,
// 2=distance, 3=velocity) inside the header strip. Column tiles span the
// header; row text uses Column.Right() as its fixed alignment anchor, so
// number columns never shift as digits change.
func OverviewColumn(r Rect, c int) Rect {
	h := OverviewHeaderRect(r)
	distW, velW := ovDistWidth(r), ovVelWidth(r)
	nameW := h.W - ovIconW - distW - velW
	switch c {
	case 0:
		return Rect{h.X, h.Y, ovIconW, h.H}
	case 1:
		return Rect{h.X + ovIconW, h.Y, nameW, h.H}
	case 2:
		return Rect{h.X + ovIconW + nameW, h.Y, distW, h.H}
	default:
		return Rect{h.X + ovIconW + nameW + distW, h.Y, velW, h.H}
	}
}

// Overview column widths (distance/velocity scale with the panel so small
// windows keep a usable name column).
func ovDistWidth(r Rect) float64 { return clamp(r.W*0.27, 64, 92) }
func ovVelWidth(r Rect) float64  { return clamp(r.W*0.21, 52, 72) }

// OverviewMaxRows returns how many data rows fit in the overview viewport.
func OverviewMaxRows(r Rect) int {
	body := OverviewBody(r)
	n := int((body.H - padSmall) / ovRowH)
	if n < 0 {
		return 0
	}
	return n
}

// OverviewRowRect returns the rect of visible data row i at scroll offset
// scroll (rows above the viewport slide up as scroll grows; drawing clips
// them to the body).
func OverviewRowRect(r Rect, i int, scroll float64) Rect {
	body := OverviewBody(r)
	return Rect{
		X: r.X + 1,
		Y: body.Y + float64(i)*ovRowH - scroll,
		W: r.W - 2,
		H: ovRowH - padSmall,
	}
}

// OverviewRowAt returns the content row index under y given the scroll
// offset and total row count, or -1.
func OverviewRowAt(r Rect, y, scroll float64, total int) int {
	body := OverviewBody(r)
	if y < body.Y || y >= body.Bottom() {
		return -1
	}
	i := int((y - body.Y + scroll) / ovRowH)
	if i < 0 || i >= total {
		return -1
	}
	return i
}

// OverviewColumnAt returns the header column index under x, or -1.
func OverviewColumnAt(r Rect, x float64) int {
	h := OverviewHeaderRect(r)
	for c := 0; c < 4; c++ {
		if OverviewColumn(r, c).Contains(x, h.Y+1) {
			return c
		}
	}
	return -1
}

// OverviewFilterButtonRect is the header filter toggle in the title bar.
func OverviewFilterButtonRect(r Rect) Rect {
	return Rect{
		X: r.Right() - minHit - padSmall,
		Y: r.Y + (titleBarH-minHit)/2,
		W: minHit,
		H: minHit,
	}
}

// filterGroups enumerates the overview filter toggles, in draw order.
var filterGroups = [...]FilterGroup{FilterShips, FilterAsteroids, FilterWrecks, FilterStations, FilterCelestials}

// filterGroupLabels are the popup row labels.
var filterGroupLabels = [...]string{"Ships", "Asteroids", "Wrecks", "Stations & Gates", "Celestials"}

// OverviewFilterPopupRect is the dropdown opened from the filter button.
func OverviewFilterPopupRect(r Rect) Rect {
	w := clamp(r.W-2, 220, 260)
	return Rect{X: r.X + 1, Y: r.Y + 1 + titleBarH, W: w, H: 2*padSmall + float64(len(filterGroups))*minHit}
}

// OverviewFilterRowRect returns row i of the filter dropdown.
func OverviewFilterRowRect(p Rect, i int) Rect {
	return Rect{X: p.X + padSmall, Y: p.Y + padSmall + float64(i)*minHit, W: p.W - 2*padSmall, H: minHit}
}

// ---------------------------------------------------------------------------
// Scrolling (pure math — tested)
// ---------------------------------------------------------------------------

// ScrollMax returns the maximum scroll offset for content/view sizes.
func ScrollMax(contentH, viewH float64) float64 {
	m := contentH - viewH
	if m < 0 {
		return 0
	}
	return m
}

// ClampScroll bounds a scroll offset to [0, ScrollMax(contentH, viewH)].
func ClampScroll(scroll, contentH, viewH float64) float64 {
	return clamp(scroll, 0, ScrollMax(contentH, viewH))
}

// ScrollbarTrack returns the reserved scrollbar strip of a body rect.
func ScrollbarTrack(body Rect) Rect {
	return Rect{X: body.Right() + 2, Y: body.Y, W: ovScrollW - 2, H: body.H}
}

// ScrollThumbRect returns the thumb rect for a track given content and
// viewport heights and the current scroll offset. The thumb keeps a
// minimum size, fills the track when everything fits, and stays inside
// the track at both scroll extremes.
func ScrollThumbRect(track Rect, contentH, viewH, scroll float64) Rect {
	if contentH <= viewH || viewH <= 0 {
		return track
	}
	th := track.H * viewH / contentH
	if min := padLarge; th < min {
		th = min
	}
	maxScroll := ScrollMax(contentH, viewH)
	frac := 0.0
	if maxScroll > 0 {
		frac = clamp(scroll/maxScroll, 0, 1)
	}
	ty := track.Y + (track.H-th)*frac
	return Rect{X: track.X, Y: ty, W: track.W, H: th}
}

// ---------------------------------------------------------------------------
// Selected-item geometry
// ---------------------------------------------------------------------------

// WarpSegRects returns the warp-to-within segmented control rects
// (0 | 10 | 50 km): fixed 40×40 segments in the row above the action grid.
func WarpSegRects(r Rect, n int) []Rect {
	x0 := r.X + 1 + pad
	gap := padSmall
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		out[i] = Rect{X: x0 + float64(i)*(minHit+gap), Y: actStart - warpSegH - padSmall, W: minHit, H: warpSegH}
	}
	return out
}

// WarpButtonRect returns the warp action button right of the range segments;
// it carries the current choice in its label ("Warp to 10 km").
func WarpButtonRect(r Rect, n int) Rect {
	segs := WarpSegRects(r, n)
	x := segs[n-1].Right() + padSmall
	return Rect{
		X: x,
		Y: actStart - warpSegH - padSmall,
		W: r.Right() - 1 - pad - x,
		H: warpSegH,
	}
}

// ActionRects returns up to 3×N button rects inside the selected panel;
// button width adapts to the panel so nothing overflows at 1024×700.
func ActionRects(r Rect, n int) []Rect {
	btnW := (r.W - 2 - 2*pad - actGap*float64(actCols-1)) / float64(actCols)
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		col := i % actCols
		row := i / actCols
		out[i] = Rect{
			X: r.X + 1 + pad + float64(col)*(btnW+actGap),
			Y: r.Y + actStart + float64(row)*(actBtnH+actGap),
			W: btnW,
			H: actBtnH,
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Target boxes (top-center lock list)
// ---------------------------------------------------------------------------

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
		out[i] = Rect{x, r.Y + padSmall, targetBoxW, targetBoxH}
		x += targetBoxW + targetGap
	}
	return out
}

// ---------------------------------------------------------------------------
// Module rack (bottom-right)
// ---------------------------------------------------------------------------

// ModuleRowLabelRect returns the row label rect ("High"/"Mid"/"Low").
func ModuleRowLabelRect(r Rect, row int) Rect {
	return Rect{r.X + modPad, r.Y + modPad + float64(row)*modRowH, modLabelW, modLabelH}
}

// ModuleCellRect returns the cell rect for slot i of the given row.
func ModuleCellRect(r Rect, row, i int) Rect {
	return Rect{
		X: r.X + modPad + modLabelW + padSmall + float64(i)*(modCell+modGap),
		Y: r.Y + modPad + float64(row)*modRowH + modLabelH,
		W: modCell,
		H: modCell,
	}
}

// ModuleMaxCells returns how many cells fit in one rack row.
func ModuleMaxCells(r Rect) int {
	avail := r.W - 2*modPad - modLabelW - padSmall
	n := int((avail + modGap) / (modCell + modGap))
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

// ---------------------------------------------------------------------------
// Tabs / station body
// ---------------------------------------------------------------------------

// TabRects lays n tabs across the top of a panel (below its title bar).
func TabRects(r Rect, n int) []Rect {
	avail := r.W - 2*padSmall
	tw := avail / float64(n)
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		out[i] = Rect{r.X + padSmall + float64(i)*tw, r.Y + titleBarH + padSmall, tw - padSmall, tabH}
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
	return Rect{
		X: r.X + pad,
		Y: r.Y + titleBarH + tabStrip + padSmall,
		W: r.W - 2*pad,
		H: r.H - titleBarH - tabStrip - 2*pad,
	}
}

// ---------------------------------------------------------------------------
// Fitting window slot rows
// ---------------------------------------------------------------------------

// FittingSlotRow returns the slot-row rect for the given class row.
func FittingSlotRow(r Rect, row int) Rect {
	return Rect{r.X + pad, r.Y + fitFirstRowY + float64(row)*fitSlotRowH, r.W - 2*pad, fitSlotRowH - pad}
}

// FittingCellRect returns slot cell i inside a fitting slot row.
func FittingCellRect(row Rect, i int) Rect {
	return Rect{
		X: row.X + fitLabelW + float64(i)*(fitCellW+fitCellGap),
		Y: row.Y + (row.H-fitCellH)/2,
		W: fitCellW,
		H: fitCellH,
	}
}

// FittingUnfitRect returns the small [x] button over a filled cell (visual
// only — a 40×40 hit target would swallow the whole cell; exempt from the
// minimum hit-area rule).
func FittingUnfitRect(cell Rect) Rect {
	return Rect{cell.Right() - 16, cell.Y + 2, 16, 16}
}

// FittingMaxCells returns how many fitting cells fit in one slot row.
func FittingMaxCells(r Rect) int {
	rowW := r.W - 2*pad
	n := int((rowW - fitLabelW) / (fitCellW + fitCellGap))
	if n < 1 {
		n = 1
	}
	return n
}

// ---------------------------------------------------------------------------
// Station tables (market / refine / hangar lists)
// ---------------------------------------------------------------------------

// StationHeadRect returns the column-header strip of a station table body.
func StationHeadRect(body Rect) Rect {
	return Rect{body.X, body.Y, body.W, stHeadH}
}

// StationRowRect returns data row i inside a station table body.
func StationRowRect(body Rect, i int) Rect {
	return Rect{body.X, body.Y + stHeadH + float64(i)*stRowH, body.W, stRowH - padSmall}
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

// RightButton returns a button of width w anchored to a row's right edge
// (full 40 px hit height).
func RightButton(row Rect, w float64) Rect {
	h := row.H - padSmall
	if h < minHit {
		h = minHit
	}
	return Rect{row.Right() - w - padSmall, row.Y, w, h}
}
