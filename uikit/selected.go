package uikit

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
)

// Orbit ranges (m) the Orbit selector cycles through, and warp landing
// distances (m) the Warp-to-within selector cycles through. Exported so
// the docs and tests can quote the exact sets.
var (
	OrbitRanges = [...]float64{2500, 5000, 10000, 20000, 50000}
	WarpRanges  = [...]float64{0, 10000, 50000}
)

// selectedPanel owns the "Selected Item" window: identity labels plus
// the command buttons that prove the enqueue seam. The two range
// controls are pure selectors (state, never commands); the Warp and
// Orbit ACTION buttons beside Approach/Stop are what enqueue, at the
// selector value the same frame — Kit.WarpRange/Kit.OrbitRange hand that
// value to the keybinds, so panel and keyboard can never disagree.
type selectedPanel struct {
	win  *widget.Window
	body *widget.Container
	name *widget.Text
	dist *widget.Text

	// kindIcon leads the name line: the selection's kind glyph at the
	// title size, hidden while nothing is selected.
	kindIcon *iconBox

	approach, warp, orbit, stop *widget.Button
	lock, dock, jump            *widget.Button
	loot, mine                  *widget.Button
	orbitRange, warpRange       *widget.Button

	// warpTip explains why Warp is disabled when the selected target
	// sits inside the warp minimum range (sim.WarpMinRange).
	warpTip     *widget.ToolTip
	warpTips    []*widget.ToolTip
	warpTipView *tipView // structured "Warp unavailable" panel
	warpTipKey  string   // reason the panel was built from ("" = none)

	orbitIdx, warpRangeIdx int // cycle positions of the two selectors
}

// selectedBands lists the button cells in every band of the body, in
// child order after the two text lines: movement (approach, warp,
// orbit, stop), context actions and the two range selectors (one
// full-width cell each).
var selectedBands = [...]int{4, 5, 1, 1}

// buildSelected constructs the Selected Item window and its handlers.
func (k *Kit) buildSelected() {
	p := k.th.Pal()
	sp := &selectedPanel{}
	k.sp = sp

	sp.name = widget.NewText(
		widget.TextOpts.Text("No selection", k.th.FaceBold(StepLarge), solid(p.Text)),
	)
	sp.dist = widget.NewText(
		widget.TextOpts.Text("", k.th.Face(StepTiny), solid(p.Dim)),
	)
	// The kind glyph leads the name line (icon box + label, one row).
	sp.kindIcon = newIconBox(IconTitle)
	nameRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(IconGap)),
		)),
	)
	nameRow.AddChild(sp.kindIcon.Widget(), sp.name)

	// Action buttons carry a 20 px icon beside their label (ebitenui's
	// TextAndImage path: graphic after the label, themed image states
	// unchanged); the two range selectors stay text-only — they are
	// state, not actions.
	newBtn := func(label, icon string, clicked func(*widget.ButtonClickedEventArgs)) *widget.Button {
		opts := []widget.ButtonOpt{
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, Px(40)),
			),
			widget.ButtonOpts.ClickedHandler(clicked),
		}
		if icon != "" {
			opts = append(opts, widget.ButtonOpts.TextAndImage(label, nil, &widget.GraphicImage{
				Idle:     icons.Get(icon, Px(IconButton), p.Text),
				Disabled: icons.Get(icon, Px(IconButton), iconColor(p.Dim, IconDisabledA)),
			}, nil))
		} else {
			opts = append(opts, widget.ButtonOpts.TextLabel(label))
		}
		return widget.NewButton(opts...)
	}
	// Core movement actions (kept from the spike). Warp and Orbit are
	// the action half of the warp/orbit pair: they enqueue at the range
	// their selector shows, never a range of their own.
	sp.approach = newBtn("Approach", "act_approach", func(*widget.ButtonClickedEventArgs) { k.approachSelected() })
	sp.warp = newBtn("Warp", "act_warp", func(*widget.ButtonClickedEventArgs) { k.warpSelected() })
	sp.orbit = newBtn("Orbit", "act_orbit", func(*widget.ButtonClickedEventArgs) { k.orbitSelected() })
	sp.stop = newBtn("Stop", "act_stop", func(*widget.ButtonClickedEventArgs) { k.stopSelected() })
	// Context actions, gated by the selection's sim.Info flags in
	// syncSelected (disabled buttons take the theme's disabled look).
	sp.lock = newBtn("Lock", "act_lock", func(*widget.ButtonClickedEventArgs) { k.lockSelected() })
	sp.dock = newBtn("Dock", "act_dock", func(*widget.ButtonClickedEventArgs) { k.dockSelected() })
	sp.jump = newBtn("Jump", "act_jump", func(*widget.ButtonClickedEventArgs) { k.jumpSelected() })
	sp.loot = newBtn("Loot", "act_loot", func(*widget.ButtonClickedEventArgs) { k.lootSelected() })
	sp.mine = newBtn("Mine", "act_mine", func(*widget.ButtonClickedEventArgs) { k.mineSelected() })
	// Pure selectors: always enabled, they only cycle their own value —
	// they never enqueue. The Warp/Orbit actions and the Z/Q keys read
	// that value back through Kit.WarpRange / Kit.OrbitRange.
	sp.orbitRange = newBtn(orbitLabel(OrbitRanges[0]), "", func(*widget.ButtonClickedEventArgs) { k.cycleOrbit() })
	sp.warpRange = newBtn(warpRangeLabel(WarpRanges[0]), "", func(*widget.ButtonClickedEventArgs) { k.cycleWarpRange() })

	// Hover hint for the Warp action while the target sits inside the
	// warp minimum range: one ToolTip for the button's lifetime, its
	// panel refreshed by syncSelected and attached only in that state
	// (the module rack's tooltip pattern).
	sp.warpTip, sp.warpTipView = newTip(k.th, TooltipModel{})
	sp.warpTips = []*widget.ToolTip{sp.warpTip}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		// The body's geometry comes from the pure selectedLayout below —
		// rows stack from the top, bands tile their row with equal gaps.
		widget.ContainerOpts.Layout(&selectedLayouter{bands: selectedBands[:]}),
	)
	sp.body = body
	body.AddChild(nameRow, sp.dist,
		sp.approach, sp.warp, sp.orbit, sp.stop,
		sp.lock, sp.dock, sp.jump, sp.loot, sp.mine,
		sp.orbitRange, sp.warpRange)

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Selected Item", k.th.FaceBold(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(Px(contentsPad)),
		})),
	))

	sp.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, Px(titleBarH)),
		widget.WindowOpts.Draggable(),
		// The two action rows must hold icon + label whole: five
		// TextAndImage buttons measure 447 px on the type grid, so the
		// content floor (mirrored by the default rect below) is 450.
		widget.WindowOpts.MinSize(Px(450), Px(240)),
		widget.WindowOpts.Location(image.Rect(Px(8), Px(436), Px(458), Px(696))),
	)
}

// ---------------------------------------------------------------------------
// Pure layout math
// ---------------------------------------------------------------------------

// selectedInventory is the content the layout works from: the measured
// heights of the two text lines, the natural width of every cell in each
// button band, the bands' natural height and the gap used between rows
// and between cells.
type selectedInventory struct {
	nameH, distH int
	bands        [][]float64
	bandH        int
	gap          int
}

// selectedLayout maps the body's inner rect onto one rect per child, in
// child order: the name line, the distance line, then every button cell
// band by band. Rows stack from the top of inner with inv.gap between
// them; the text lines keep their measured height and the bands share
// whatever height is left, capped at their natural height — so every rect
// stays inside inner even at the 450×240 minimum. Cells within a band
// scale proportionally to their natural widths with exactly inv.gap
// between them, which tiles the band edge to edge.
func selectedLayout(inner image.Rectangle, inv selectedInventory) []image.Rectangle {
	gap := inv.gap
	if gap < 0 {
		gap = 0
	}
	rows := 2 + len(inv.bands)
	avail := inner.Dy() - (inv.nameH + inv.distH) - gap*(rows-1)
	bandH := 0
	if len(inv.bands) > 0 {
		bandH = avail / len(inv.bands)
		if bandH > inv.bandH {
			bandH = inv.bandH
		}
		if bandH < 0 {
			bandH = 0
		}
	}

	rects := make([]image.Rectangle, 0, rows)
	y := inner.Min.Y
	place := func(h int) image.Rectangle {
		if y > inner.Max.Y {
			y = inner.Max.Y
		}
		if h > inner.Max.Y-y {
			h = inner.Max.Y - y
		}
		if h < 0 {
			h = 0
		}
		r := image.Rect(inner.Min.X, y, inner.Max.X, y+h)
		y += h + gap
		return r
	}
	rects = append(rects, place(inv.nameH), place(inv.distH))
	for _, cells := range inv.bands {
		if len(cells) == 0 {
			continue
		}
		rects = append(rects, splitBand(place(bandH), cells, gap)...)
	}
	return rects
}

// splitBand tiles one band across its row: cell widths follow their
// natural sizes, rounded cumulatively so the last cell lands exactly on
// the band's right edge, and every gap between cells equals gap.
func splitBand(band image.Rectangle, natural []float64, gap int) []image.Rectangle {
	n := len(natural)
	if n == 0 {
		return nil
	}
	avail := band.Dx() - gap*(n-1)
	if avail < 0 {
		avail = 0
	}
	sum := 0.0
	for _, w := range natural {
		if w > 0 {
			sum += w
		}
	}
	cells := make([]image.Rectangle, n)
	prev := 0
	acc := 0.0
	for i, w := range natural {
		end := avail
		switch {
		case i == n-1:
			end = avail // the grid lands exactly on the edge
		case sum > 0:
			if w > 0 {
				acc += w
			}
			end = int(math.Round(float64(avail) * acc / sum))
		default: // no measurable widths: split evenly
			end = (i + 1) * avail / n
		}
		if end < prev {
			end = prev
		}
		x := band.Min.X + prev + i*gap
		cells[i] = image.Rect(x, band.Min.Y, x+end-prev, band.Max.Y)
		prev = end
	}
	return cells
}

// selectedLayouter drives the Selected Item body from selectedLayout: it
// measures the children (the inventory), hands the body's inner rect to
// the pure function and pushes the result back onto the children, which
// are the two text lines followed by selectedBands' cells in order.
type selectedLayouter struct {
	bands []int
}

// inventory measures the children into selectedLayout's input.
func (l *selectedLayouter) inventory(children []widget.PreferredSizeLocateableWidget) selectedInventory {
	// gap holds SCALED px (Px once here): splitBand and the row math all
	// read this field, so they must not scale it again.
	inv := selectedInventory{gap: Px(contentsPad)}
	if len(children) > 0 {
		_, inv.nameH = children[0].PreferredSize()
	}
	if len(children) > 1 {
		_, inv.distH = children[1].PreferredSize()
	}
	idx := 2
	for _, n := range l.bands {
		cells := make([]float64, 0, n)
		for j := 0; j < n && idx < len(children); j++ {
			w, h := children[idx].PreferredSize()
			cells = append(cells, float64(w))
			if h > inv.bandH {
				inv.bandH = h
			}
			idx++
		}
		inv.bands = append(inv.bands, cells)
	}
	return inv
}

// PreferredSize implements widget.Layouter: the natural stack of rows
// and the widest child, plus the panel padding.
func (l *selectedLayouter) PreferredSize(children []widget.PreferredSizeLocateableWidget) (int, int) {
	inv := l.inventory(children)
	h := inv.nameH + inv.distH + len(inv.bands)*inv.bandH + inv.gap*(1+len(inv.bands)) + 2*inv.gap
	w := 0
	for _, ch := range children {
		if cw, _ := ch.PreferredSize(); cw > w {
			w = cw
		}
	}
	return w + 2*inv.gap, h
}

// Layout implements widget.Layouter: selectedLayout's rects are in child
// order, so each child takes its own.
func (l *selectedLayouter) Layout(children []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	rects := selectedLayout(rect.Inset(Px(contentsPad)), l.inventory(children))
	for i, r := range rects {
		if i >= len(children) {
			return
		}
		children[i].SetLocation(r)
	}
}

// ---------------------------------------------------------------------------
// Selection sync
// ---------------------------------------------------------------------------

// syncSelected refreshes the labels and button enablement from the
// current selection: every action button is disabled while nothing is
// selected, and the context actions additionally require their
// sim.Info flag (dock/jump/loot/mine/warp), mirroring ui.ActionsFor.
// Warp additionally refuses targets inside sim.WarpMinRange (with the
// hover hint saying so). The two range selectors stay enabled — they
// are state, they never enqueue.
func (k *Kit) syncSelected() {
	sp := k.sp
	info, ok := k.cv.Info(k.selected)
	name, dist := "No selection", ""
	if ok {
		name = info.Name
		dist = formatDistance(info.Distance) + "  ·  " + kindLabel(info.Kind)
		sp.kindIcon.Set(KindIcon(info.Kind), k.th.Pal().Text)
	} else {
		sp.kindIcon.Set("", color.RGBA{})
	}
	if sp.name.Label != name || sp.dist.Label != dist {
		sp.name.Label, sp.dist.Label = name, dist
		// The line heights feed the layout: without a relayout the
		// distance line keeps its empty-label rect (zero size, sitting
		// over the first button row) and draws on top of the buttons.
		sp.body.RequestRelayout()
	}
	noSel := !ok
	// Too close to warp: the drive cannot engage inside WarpMinRange, so
	// the action is gated off and the tooltip names the reason.
	tooClose := ok && info.CanWarp && info.Distance < sim.WarpMinRange
	sp.approach.GetWidget().Disabled = noSel
	sp.stop.GetWidget().Disabled = noSel
	sp.orbit.GetWidget().Disabled = noSel
	sp.warp.GetWidget().Disabled = noSel || !info.CanWarp || tooClose
	sp.lock.GetWidget().Disabled = noSel || !canLock(info.Kind)
	sp.dock.GetWidget().Disabled = noSel || !info.CanDock
	sp.jump.GetWidget().Disabled = noSel || !info.CanJump
	sp.loot.GetWidget().Disabled = noSel || !info.CanLoot
	sp.mine.GetWidget().Disabled = noSel || !info.CanMine
	if tooClose {
		reason := "Target within warp minimum range (" + kmLabel(sim.WarpMinRange) + ")"
		if sp.warpTipKey != reason {
			sp.warpTipKey = reason
			sp.warpTipView.SetModel(TooltipModel{
				Icon:  "act_warp",
				Title: "Warp unavailable",
				Sub:   reason,
			})
		}
		sp.warp.GetWidget().ToolTips = sp.warpTips
	} else {
		sp.warpTipKey = ""
		sp.warp.GetWidget().ToolTips = nil
	}
}

// kmLabel renders a metre distance as the kilometre figure quoted in
// hints ("150 km" for sim.WarpMinRange) — mirrors sim.kmLabel so the
// panel and the refusal log line word it identically.
func kmLabel(m float64) string {
	return strconv.FormatFloat(m/1000, 'f', -1, 64) + " km"
}

// canLock mirrors ui.canLock: the entity kinds that can be targeted.
func canLock(kind string) bool {
	switch kind {
	case sim.KindShip, sim.KindAsteroid, sim.KindWreck:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Range controls
// ---------------------------------------------------------------------------

// orbitLabel renders the Orbit selector: "Orbit range: 2,500 m »".
func orbitLabel(rng float64) string {
	return "Orbit range: " + FormatInt(int64(rng)) + " m »"
}

// warpRangeLabel renders the warp-to-within selector:
// "Warp to within: 10,000 m »".
func warpRangeLabel(rng float64) string {
	return "Warp to within: " + FormatInt(int64(rng)) + " m »"
}

// WarpRange is the warp landing distance the Warp action and the Z key
// both act at: the value the Selected Item warp selector currently
// shows. One source of truth for the panel buttons and the keybind.
func (k *Kit) WarpRange() float64 { return WarpRanges[k.sp.warpRangeIdx] }

// OrbitRange is the orbit range the Orbit action and the Q key both act
// at: the value the Selected Item orbit selector currently shows.
func (k *Kit) OrbitRange() float64 { return OrbitRanges[k.sp.orbitIdx] }

// cycleOrbit advances the orbit selector (wraparound). Pure state: it
// enqueues nothing, ever — the Orbit button and Q act at OrbitRange().
func (k *Kit) cycleOrbit() {
	sp := k.sp
	sp.orbitIdx = (sp.orbitIdx + 1) % len(OrbitRanges)
	sp.orbitRange.SetText(orbitLabel(OrbitRanges[sp.orbitIdx]))
}

// cycleWarpRange advances the warp-to-within selector (wraparound).
// Pure state: it enqueues nothing, ever — the Warp button and Z act at
// WarpRange().
func (k *Kit) cycleWarpRange() {
	sp := k.sp
	sp.warpRangeIdx = (sp.warpRangeIdx + 1) % len(WarpRanges)
	sp.warpRange.SetText(warpRangeLabel(WarpRanges[sp.warpRangeIdx]))
}

// ---------------------------------------------------------------------------
// Action seam
// ---------------------------------------------------------------------------

// approachSelected emits Approach for the selected entity.
func (k *Kit) approachSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(Approach{Entity: k.selected})
}

// warpSelected emits WarpTo for the selected entity; the executor reads
// the landing range from the warp selector at press (Kit.WarpRange — the
// same value Z uses). The button is disabled without a valid, warpable,
// distant enough target; this guard keeps the seam safe when driven
// directly.
func (k *Kit) warpSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(WarpTo{Entity: k.selected})
}

// orbitSelected emits Orbit for the selected entity; the executor reads
// the range from the orbit selector at press (Kit.OrbitRange — the same
// value Q uses).
func (k *Kit) orbitSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(Orbit{Entity: k.selected})
}

// stopSelected emits Stop (cancels movement intent).
func (k *Kit) stopSelected() {
	k.sink.Do(Stop{})
}

// lockSelected emits Lock for the selected entity.
func (k *Kit) lockSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(Lock{Entity: k.selected})
}

// dockSelected emits Dock at the selected station.
func (k *Kit) dockSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(Dock{Entity: k.selected})
}

// jumpSelected emits Jump through the selected gate.
func (k *Kit) jumpSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(Jump{Entity: k.selected})
}

// lootSelected emits Loot (empty item = everything) at the wreck.
func (k *Kit) lootSelected() {
	if k.selected.IsZero() {
		return
	}
	k.sink.Do(Loot{Entity: k.selected})
}

// mineSelected fires the first fitted mining laser at the selected
// asteroid (mirrors ui.UI.mineSelected): the slot resolves at press and
// rides the intent.
func (k *Kit) mineSelected() {
	if k.selected.IsZero() {
		return
	}
	info, ok := k.cv.Info(k.selected)
	if !ok || info.Kind != sim.KindAsteroid {
		return
	}
	slot, ok := k.firstMiningSlot()
	if !ok {
		return
	}
	k.sink.Do(Mine{Entity: k.selected, Slot: slot})
}

// firstMiningSlot finds the first fitted mining laser.
func (k *Kit) firstMiningSlot() (sim.SlotRef, bool) {
	fit, ok := k.cv.Fitting(k.cv.Player())
	if !ok {
		return sim.SlotRef{}, false
	}
	for _, class := range []string{content.SlotHigh, content.SlotMid} {
		for i, id := range fit.Defs(class) {
			if def, ok := k.cv.Defs().Module(id); ok && def.Kind == content.KindMiningLaser {
				return sim.SlotRef{Class: class, Index: i}, true
			}
		}
	}
	return sim.SlotRef{}, false
}
