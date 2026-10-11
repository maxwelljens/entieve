package uikit

import (
	"image"
	"math"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// Orbit ranges (m) the Orbit control cycles through, and warp landing
// distances (m) the Warp-to-within control cycles through. Note these
// differ from game's keybind cycles (game.OrbitRanges, warpRangeMeters):
// the panel offers the wider EVE-style set (reported as a GAMEPLAY delta).
var (
	orbitRanges = [...]float64{2500, 5000, 10000, 20000, 50000}
	warpRanges  = [...]float64{0, 10000, 50000}
)

// selectedPanel owns the "Selected Item" window: identity labels plus
// the command buttons that prove the enqueue seam.
type selectedPanel struct {
	win  *widget.Window
	body *widget.Container
	name *widget.Text
	dist *widget.Text

	approach, warp, stop *widget.Button
	lock, dock, jump     *widget.Button
	loot, mine           *widget.Button
	orbit, warpRange     *widget.Button

	orbitIdx, warpRangeIdx int // cycle positions of the two range controls
}

// selectedBands lists the button cells in every band of the body, in
// child order after the two text lines: movement, context actions and
// the two range controls (one full-width cell each).
var selectedBands = [...]int{3, 5, 1, 1}

// buildSelected constructs the Selected Item window and its handlers.
func (k *Kit) buildSelected() {
	p := k.palette
	sp := &selectedPanel{}
	k.sp = sp

	sp.name = widget.NewText(
		widget.TextOpts.Text("No selection", facePtr(k.faces.large), solid(p.Text)),
	)
	sp.dist = widget.NewText(
		widget.TextOpts.Text("", facePtr(k.faces.tiny), solid(p.Dim)),
	)

	newBtn := func(label string, clicked func(*widget.ButtonClickedEventArgs)) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, 40),
			),
			widget.ButtonOpts.ClickedHandler(clicked),
		)
	}
	// Core movement actions (kept from the spike).
	sp.approach = newBtn("Approach", func(*widget.ButtonClickedEventArgs) { k.approachSelected() })
	sp.warp = newBtn("Warp to 10 km", func(*widget.ButtonClickedEventArgs) { k.warpSelected() })
	sp.stop = newBtn("Stop", func(*widget.ButtonClickedEventArgs) { k.stopSelected() })
	// Context actions, gated by the selection's sim.Info flags in
	// syncSelected (disabled buttons take the theme's disabled look).
	sp.lock = newBtn("Lock", func(*widget.ButtonClickedEventArgs) { k.lockSelected() })
	sp.dock = newBtn("Dock", func(*widget.ButtonClickedEventArgs) { k.dockSelected() })
	sp.jump = newBtn("Jump", func(*widget.ButtonClickedEventArgs) { k.jumpSelected() })
	sp.loot = newBtn("Loot", func(*widget.ButtonClickedEventArgs) { k.lootSelected() })
	sp.mine = newBtn("Mine", func(*widget.ButtonClickedEventArgs) { k.mineSelected() })
	// Range selectors: always enabled (they pick a range, they do not act
	// on the selection); the enqueue happens only against a live target.
	sp.orbit = newBtn(orbitLabel(orbitRanges[0]), func(*widget.ButtonClickedEventArgs) { k.cycleOrbit() })
	sp.warpRange = newBtn(warpRangeLabel(warpRanges[0]), func(*widget.ButtonClickedEventArgs) { k.cycleWarpRange() })

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		// The body's geometry comes from the pure selectedLayout below —
		// rows stack from the top, bands tile their row with equal gaps.
		widget.ContainerOpts.Layout(&selectedLayouter{bands: selectedBands[:]}),
	)
	sp.body = body
	body.AddChild(sp.name, sp.dist,
		sp.approach, sp.warp, sp.stop,
		sp.lock, sp.dock, sp.jump, sp.loot, sp.mine,
		sp.orbit, sp.warpRange)

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Selected Item", facePtr(k.faces.base), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	))

	sp.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		// Wide enough for the two button rows: the layout never wraps, so
		// the min size must keep the content inside the panel.
		widget.WindowOpts.MinSize(300, 240),
		widget.WindowOpts.Location(image.Rect(8, 436, 388, 696)),
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
// stays inside inner even at the 300×240 minimum. Cells within a band
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
	inv := selectedInventory{gap: contentsPad}
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
	rects := selectedLayout(rect.Inset(contentsPad), l.inventory(children))
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
// The two range controls stay enabled — they pick a range, and the
// enqueue is gated in their own handlers.
func (k *Kit) syncSelected() {
	sp := k.sp
	info, ok := k.Sim.Info(k.selected)
	name, dist := "No selection", ""
	if ok {
		name = info.Name
		dist = formatDistance(info.Distance) + "  ·  " + kindLabel(info.Kind)
	}
	if sp.name.Label != name || sp.dist.Label != dist {
		sp.name.Label, sp.dist.Label = name, dist
		// The line heights feed the layout: without a relayout the
		// distance line keeps its empty-label rect (zero size, sitting
		// over the first button row) and draws on top of the buttons.
		sp.body.RequestRelayout()
	}
	noSel := !ok
	sp.approach.GetWidget().Disabled = noSel
	sp.stop.GetWidget().Disabled = noSel
	sp.warp.GetWidget().Disabled = noSel || !info.CanWarp
	sp.lock.GetWidget().Disabled = noSel || !canLock(info.Kind)
	sp.dock.GetWidget().Disabled = noSel || !info.CanDock
	sp.jump.GetWidget().Disabled = noSel || !info.CanJump
	sp.loot.GetWidget().Disabled = noSel || !info.CanLoot
	sp.mine.GetWidget().Disabled = noSel || !info.CanMine
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

// orbitLabel renders the Orbit control: "Orbit: 2,500 m ▸".
func orbitLabel(rng float64) string {
	return "Orbit: " + groupInt(int64(rng)) + " m ▸"
}

// warpRangeLabel renders the warp-to-within control:
// "Warp to within: 10,000 m ▸".
func warpRangeLabel(rng float64) string {
	return "Warp to within: " + groupInt(int64(rng)) + " m ▸"
}

// cycleOrbit picks the next orbit range (wraparound) and, when the
// selection resolves on the grid, enqueues CmdOrbit at that range.
func (k *Kit) cycleOrbit() {
	sp := k.sp
	sp.orbitIdx = (sp.orbitIdx + 1) % len(orbitRanges)
	rng := orbitRanges[sp.orbitIdx]
	sp.orbit.SetText(orbitLabel(rng))
	if k.selected.IsZero() {
		return
	}
	if _, ok := k.Sim.Info(k.selected); !ok {
		return
	}
	k.enqueue(sim.CmdOrbit{Target: k.selected, Range: rng})
}

// cycleWarpRange picks the next warp landing distance (wraparound) and,
// when the selection is warpable, enqueues CmdWarpTo at that range —
// the same Info.CanWarp gate the Warp button uses.
func (k *Kit) cycleWarpRange() {
	sp := k.sp
	sp.warpRangeIdx = (sp.warpRangeIdx + 1) % len(warpRanges)
	rng := warpRanges[sp.warpRangeIdx]
	sp.warpRange.SetText(warpRangeLabel(rng))
	info, ok := k.Sim.Info(k.selected)
	if k.selected.IsZero() || !ok || !info.CanWarp {
		return
	}
	k.enqueue(sim.CmdWarpTo{Target: k.selected, Range: rng})
}

// ---------------------------------------------------------------------------
// Action seam (unchanged behaviour)
// ---------------------------------------------------------------------------

// approachSelected enqueues CmdApproach for the selected entity.
func (k *Kit) approachSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdApproach{Target: k.selected})
}

// warpSelected enqueues a 10 km warp to the selected entity.
func (k *Kit) warpSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdWarpTo{Target: k.selected, Range: 10000})
}

// stopSelected enqueues CmdStop (cancels movement intent).
func (k *Kit) stopSelected() {
	k.enqueue(sim.CmdStop{})
}

// lockSelected enqueues CmdLock for the selected entity.
func (k *Kit) lockSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdLock{Target: k.selected})
}

// dockSelected enqueues CmdDock at the selected station.
func (k *Kit) dockSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdDock{Station: k.selected})
}

// jumpSelected enqueues CmdJump through the selected gate.
func (k *Kit) jumpSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdJump{Gate: k.selected})
}

// lootSelected enqueues CmdLoot (empty item = everything) at the wreck.
func (k *Kit) lootSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdLoot{Wreck: k.selected})
}

// mineSelected fires the first fitted mining laser at the selected
// asteroid (mirrors ui.UI.mineSelected).
func (k *Kit) mineSelected() {
	if k.selected.IsZero() {
		return
	}
	info, ok := k.Sim.Info(k.selected)
	if !ok || info.Kind != sim.KindAsteroid {
		return
	}
	slot, ok := k.firstMiningSlot()
	if !ok {
		return
	}
	k.enqueue(sim.CmdMine{Asteroid: k.selected, Slot: slot})
}

// firstMiningSlot finds the first fitted mining laser.
func (k *Kit) firstMiningSlot() (sim.SlotRef, bool) {
	fit, ok := k.Sim.Fittings.Get(k.Sim.Player())
	if !ok {
		return sim.SlotRef{}, false
	}
	for _, class := range []string{content.SlotHigh, content.SlotMid} {
		for i, id := range fit.Defs(class) {
			if def, ok := k.Content.Module(id); ok && def.Kind == content.KindMiningLaser {
				return sim.SlotRef{Class: class, Index: i}, true
			}
		}
	}
	return sim.SlotRef{}, false
}
