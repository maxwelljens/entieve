package uikit

import (
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Rack geometry: three slot rows of cells sized so the widest row (high 4
// slots is the widest in the hull roster) fits the 1024×700 minimum window
// beside the centered HUD. Every cell is at least the 40 px minimum hit
// area in both axes.
const (
	rackRows     = 3                      // high / mid / low
	rackMaxCells = 4                      // widest slot row across every hull
	rackCellW    = 50                     // cell width
	rackCellH    = 40                     // cell height (minimum hit area)
	rackNameH    = 19                     // name line under a tile (fits StepTiny line box)
	rackCycleH   = 4                      // cycle bar height under a cell
	rackLabelW   = 30                     // "High"/"Mid"/"Low" label column
	rackCycleMax = 1000                   // cycle bar resolution (0.1 % steps)
	rackRowGap   = 8                      // gap between slot rows
	rackTipDelay = 300 * time.Millisecond // hover time before the tooltip
	// rackIconSpacing is ebitenui's fixed spacing between a graphic
	// button's text and graphic (widget/button.go hardcodes it).
	rackIconSpacing = 10
	// rackIconPad is the row padding that puts the glyph back in the
	// tile's centre: the graphic row runs [empty text][spacing][glyph],
	// so the row's far side has to pay for both the text box and the
	// spacing (paddingRight = textWidth + rackIconSpacing). The empty
	// text measures rackIconEmptyW in the shipped theme;
	// TestIconsCaptureFrame pins the result to the drawn pixels.
	rackIconPad    = 20
	rackIconEmptyW = 10
)

// rackClasses and rackRowLabels index the three slot rows.
var (
	rackClasses   = [rackRows]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	rackRowLabels = [rackRows]string{"High", "Mid", "Low"}
)

// Rack is the bottom-right module rack: one cell per slot showing the
// module short name, an amber glow while active, a cycle progress bar and a
// hover tooltip with the module's key stats. Left-clicking a cell toggles
// the module (enqueueing CmdActivate/CmdDeactivate; empty slots are inert);
// a second click inside the double-click window shows Show Info instead of
// toggling — the click that completes the gesture is Show Info, strictly
// (the old toggle+info dual gesture is gone; a lone click still toggles
// exactly once). Right-clicking a cell opens its context menu from the
// cell's declared RowSpec — Show Info, the loaded charge as a second ref,
// and Cycle charge, which walks the old direct right-click's CmdSetAmmo
// step (Auto → the weapon's ammo-class charges sitting in cargo → Auto).
type Rack struct {
	th        Theme
	cv        ContentView
	as        ActionSink
	container *widget.Container
	cells     [rackRows][rackMaxCells]*rackCell
	ammoPref  map[sim.SlotRef]content.ItemID
	measure   func(string) float64
	// pick, when set (game wires Kit.Selected), names the entity the player
	// currently has selected so click-activation targets it — the same
	// source the F-key module toggles use — before falling back to the
	// first suitable lock.
	pick func() ecs.Entity

	// dbl is the cells' shared double-click state (the installer's
	// half): a repeat click on the same module's ref inside the window
	// shows info and skips the toggle.
	dbl RowDouble
}

// rackCell is one slot: a cell column (icon tile + name line + cycle bar)
// plus its tooltip widgets. Cells are persistent, so a fitting change only
// rewrites their name, glyph, spec (menu + tooltip data) and enabled state.
// active mirrors ModuleRuntime.Active for the click toggle (the visual
// rides the button's Disabled state: dim cell while off, amber cell while
// running).
type rackCell struct {
	slot   sim.SlotRef
	root   *widget.Container
	btn    *widget.Button
	name   *widget.Text
	bar    *widget.ProgressBar
	tip    rowTip           // structured hover tooltip (built on first fit)
	spec   RowSpec          // declared interactions, rebuilt by syncCell
	module content.ModuleID // module currently shown ("" = empty slot)
	icon   string           // glyph the tile currently shows ("" = none)
	active bool             // runtime Active at the last Sync
}

// NewRack builds the module rack over the given seams. Its container
// anchors bottom-right in an anchor-layout root; Sync refreshes every cell
// from the player's fitting.
func NewRack(env Seams) *Rack {
	p := env.Pal()
	faceTiny := env.Face(StepTiny)
	r := &Rack{th: env, cv: env, as: env, ammoPref: map[sim.SlotRef]content.ItemID{}}
	r.measure = func(s string) float64 {
		w, _ := text.Measure(s, *faceTiny, 0)
		return w
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(Px(rackRowGap)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	for row := 0; row < rackRows; row++ {
		label := widget.NewText(
			widget.TextOpts.Text(rackRowLabels[row], faceTiny, solid(p.Head)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(Px(rackLabelW), 0)),
		)
		rowBox := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(Px(contentsPad)),
			)),
		)
		rowBox.AddChild(label)
		for idx := 0; idx < rackMaxCells; idx++ {
			cell := r.newCell(row, idx)
			r.cells[row][idx] = cell
			rowBox.AddChild(cell.root)
		}
		body.AddChild(rowBox)
	}

	titleText := widget.NewText(
		widget.TextOpts.Text("Modules", env.FaceBold(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(Px(contentsPad)),
		})),
	)
	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, Px(titleBarH)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	title.AddChild(titleText)

	r.container = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionEnd,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
			Padding:            &widget.Insets{Right: Px(10), Bottom: Px(10)},
		})),
	)
	r.container.AddChild(title, body)
	return r
}

// newCell builds one slot cell: a 50×40 icon tile (the module's class
// glyph; right-click cycles a weapon's charge) over its name line and the
// cycle progress bar. The cell's structured hover tooltip is built on its
// first fit (see rowTip). The glyph lives in the tile and the name under
// it, so the label keeps the full cell width — an icon beside it would
// push the row past the HUD at the 1024×700 minimum.
func (r *Rack) newCell(row, idx int) *rackCell {
	p := r.th.Pal()
	cell := &rackCell{slot: sim.SlotRef{Class: rackClasses[row], Index: idx}}

	// The tile carries the glyph only: ebitenui lays a graphic button out
	// as [text][graphic] with a fixed 10 px spacing, so the graphic
	// padding puts the (empty) text cell first and centers the glyph —
	// paddingRight = paddingLeft + spacing is the centering condition.
	// TestRackTileIconIsCentred pins the result to the tile.
	cell.btn = widget.NewButton(
		widget.ButtonOpts.Image(rackCellImages(p)),
		widget.ButtonOpts.Graphic(&widget.GraphicImage{
			Idle:     rackGlyph("", color.RGBA{}),
			Disabled: rackGlyph("", color.RGBA{}),
		}),
		widget.ButtonOpts.GraphicPadding(widget.Insets{Left: 0, Right: Px(rackIconPad)}),
		widget.ButtonOpts.TextPadding(&widget.Insets{Left: Px(3), Right: Px(3), Top: Px(2), Bottom: Px(2)}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(Px(rackCellW), Px(rackCellH)),
			widget.WidgetOpts.MouseButtonClickedHandler(func(a *widget.WidgetMouseButtonClickedEventArgs) {
				switch a.Button {
				case ebiten.MouseButtonLeft:
					if r.dbl.Click(r.cv, r.as, cell.spec) {
						return // double-click: Show Info opened, the click does not toggle
					}
					r.toggleModule(cell)
				case ebiten.MouseButtonRight:
					openRowMenu(r.cv, r.menuSink(), cell.spec, ClickPoint(a))
				}
			}),
		),
	)
	// The name line under the tile: the same click seam as the tile, so
	// the whole cell stays clickable.
	cell.name = widget.NewText(
		widget.TextOpts.Text("", r.th.Face(StepTiny), solid(p.Dim)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, Px(rackNameH)),
			widget.WidgetOpts.MouseButtonClickedHandler(func(a *widget.WidgetMouseButtonClickedEventArgs) {
				switch a.Button {
				case ebiten.MouseButtonLeft:
					if r.dbl.Click(r.cv, r.as, cell.spec) {
						return // double-click: Show Info opened, the click does not toggle
					}
					r.toggleModule(cell)
				case ebiten.MouseButtonRight:
					openRowMenu(r.cv, r.menuSink(), cell.spec, ClickPoint(a))
				}
			}),
		),
	)
	cell.bar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, rackCycleMax, 0),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, Px(rackCycleH))),
	)
	cell.root = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
	)
	cell.root.AddChild(cell.btn, cell.name, cell.bar)
	return cell
}

// rackGlyph resolves one tile image: the module's class glyph in the given
// tint ("" = an empty slot, which gets a transparent box of the same size
// so no cell ever changes size when a module is fitted).
func rackGlyph(name string, tint color.RGBA) *ebiten.Image {
	// IconButton is a design unit: the glyph scales with the cell.
	if name == "" {
		return icons.Get(icons.Fallback, Px(IconButton), color.RGBA{})
	}
	return icons.Get(name, Px(IconButton), tint)
}

// rackCellImages builds the cell chrome: an amber-tinted glow for active
// modules and a dimmed cell for inactive/empty slots (the button's Disabled
// state picks the look, so the theme's dim label rides along).
func rackCellImages(p Palette) *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:         bordered(nrgba(p.Accent, 60), solid(p.Accent), 1),
		Hover:        bordered(nrgba(p.Accent, 95), solid(p.Accent), 1),
		Pressed:      bordered(nrgba(p.Accent, 110), nrgba(p.Accent, 220), 1),
		PressedHover: bordered(nrgba(p.Accent, 110), solid(p.Accent), 1),
		Disabled:     bordered(nrgba(p.Cell, 140), nrgba(p.Border, 110), 1),
	}
}

// Container returns the rack's root widget for the interface root (anchor
// layout).
func (r *Rack) Container() *widget.Container { return r.container }

// SetTargetPicker installs the source of the player's current selection so
// click-activation targets the selected entity first (see clickTarget).
func (r *Rack) SetTargetPicker(fn func() ecs.Entity) { r.pick = fn }

// Sync refreshes every cell — module, active state, cycle progress and
// tooltip — from the player's fitting. Call it once per frame before Update.
func (r *Rack) Sync() {
	s := r.cv
	if s == nil {
		return
	}
	player := s.Player()
	fit, hasFit := s.Fitting(player)
	now := s.Time()

	// Slot counts: the hull's slots (empty cells still show), capped at the
	// cells the rack was built with — mirrors ui's ModuleMaxCells clamp.
	var slots content.Slots
	if sh, ok := s.ShipState(player); ok {
		if def, ok := r.cv.Defs().Ship(sh.Hull); ok {
			slots = def.Slots
		}
	}
	want := [rackRows]int{slots.High, slots.Mid, slots.Low}
	filled := [rackRows]int{}
	if hasFit {
		filled = [rackRows]int{len(fit.High), len(fit.Mid), len(fit.Low)}
	}
	for row := 0; row < rackRows; row++ {
		count := max(want[row], filled[row])
		if count > rackMaxCells {
			count = rackMaxCells
		}
		for idx, cell := range r.cells[row] {
			shown := idx < count
			showWidget(cell.root.GetWidget(), shown)
			if !shown {
				continue
			}
			var id content.ModuleID
			var rt *sim.ModuleRuntime
			if hasFit {
				if defs := fit.Defs(rackClasses[row]); idx < len(defs) {
					id = defs[idx]
				}
				if rts := fit.Runtimes(rackClasses[row]); idx < len(rts) {
					rt = &rts[idx]
				}
			}
			r.syncCell(cell, id, rt, now)
		}
	}
}

// syncCell rewrites one cell's label, tooltip, glow and cycle bar.
func (r *Rack) syncCell(cell *rackCell, id content.ModuleID, rt *sim.ModuleRuntime, now float64) {
	var def *content.ModuleDef
	if id != "" {
		def, _ = r.cv.Defs().Module(id)
	}
	active := rt != nil && rt.Active
	progress := 0.0
	if def != nil && active && rt.CycleEnds > 0 && def.ActivationTime > 0 {
		// Same expression as the legacy rack: position inside the current
		// cycle window (CycleEnds - ActivationTime .. CycleEnds).
		progress = hudFrac01((now - (rt.CycleEnds - def.ActivationTime)) / def.ActivationTime)
	}

	// Rewriting the label every frame is cheap (Text.measure early-returns
	// on an unchanged label) and keeps the empty-slot case honest without
	// a separate "first sync" flag; the tooltip rebuilds only when its
	// change key moves (module, charge or cargo count).
	cell.module = id
	cell.active = active
	// Glyph + name: the tile shows the module's class, the line under it
	// keeps the short name (and the click seam) exactly where it was.
	icon := ""
	if def != nil {
		icon = ModuleIcon(def.Kind)
	}
	if cell.icon != icon {
		cell.icon = icon
		cell.btn.SetGraphicImage(&widget.GraphicImage{
			Idle:     rackGlyph(icon, r.th.Pal().Text),
			Disabled: rackGlyph(icon, iconColor(r.th.Pal().Dim, IconDisabledA)),
		})
	}
	cell.name.Label = r.shortName(def)
	nameCol := r.th.Pal().Dim
	if active {
		nameCol = r.th.Pal().Text
	}
	cell.name.SetColor(nameCol)
	// Declared interactions: the menu and double-click read cell.spec at
	// fire time, the tooltip attaches from the same data (rebuilding only
	// when its change key moves).
	cell.spec = r.cellSpec(cell, def)
	cell.tip.set(r.th, cell.btn.GetWidget(), cell.spec.Tip.Key, cell.spec.Tip.Build)
	cell.btn.GetWidget().Disabled = !active
	cell.bar.SetCurrent(int(progress * rackCycleMax))
}

// showWidget shows or hides w only on a state change (a visibility flip
// requests a parent relayout, so Sync must not spam it).
func showWidget(w *widget.Widget, show bool) {
	want := widget.Visibility_Show
	if !show {
		want = widget.Visibility_Hide
	}
	if w.GetVisibility() != want {
		w.SetVisibility(want)
	}
}

// shortName is the label drawn in a cell: the module's first word, ellipsized
// to the cell (empty slots draw nothing).
func (r *Rack) shortName(def *content.ModuleDef) string {
	if def == nil {
		return ""
	}
	name := def.Name
	if i := strings.IndexByte(name, ' '); i > 0 {
		name = name[:i]
	}
	return Ellipsize(name, float64(Px(rackCellW-6)), r.measure)
}

// cellSpec is one fitted cell's declared interactions (RowSpec): Show
// Info on the module first, the preferred charge as the secondary ref,
// then the separator and Cycle charge — gated exactly where it could
// no-op. An empty slot declares nothing: no menu, no tooltip, and its
// double-click (like every click) never leaves the toggle's hands.
func (r *Rack) cellSpec(cell *rackCell, def *content.ModuleDef) RowSpec {
	if def == nil {
		return RowSpec{}
	}
	spec := RowSpec{}
	ref := ModuleRef(cell.module)
	spec.Ref = &ref
	if id := r.ammoPref[cell.slot]; id != "" {
		secondary := ItemRef(id)
		spec.SecRef = &secondary
	}
	if def.Weapon != nil && def.Weapon.AmmoClass != "" {
		next, advances := cycleNext(r.ammoCandidates(def.Weapon.AmmoClass), r.ammoPref[cell.slot])
		cycleable := advances && r.as != nil
		spec.Actions = []RowAction{{
			Label:    "Cycle charge",
			Disabled: !cycleable,
			Intent:   CycleCharge{Slot: cell.slot, Item: next},
		}}
	}
	key, build := r.cellTip(def, cell.slot)
	spec.Tip = TipData{Key: key, Build: build}
	return spec
}

// menuSink is the sink the row menu dispatches through: a Cycle charge
// intent also advances the rack's own preferred-charge mirror on its way
// out. The mirror is presentation state the sim keeps private (its pref
// is unexported), and both the cell's SecRef and the cycle order read it
// — so the rack updates it exactly when its own menu fires the intent.
func (r *Rack) menuSink() ActionSink {
	return sinkFunc(func(i Intent) {
		if cc, ok := i.(CycleCharge); ok {
			r.ammoPref[cc.Slot] = cc.Item
		}
		r.as.Do(i)
	})
}

// cellTip builds one fitted cell's tooltip: the shared module model plus
// what the rack alone knows — the slot, the preferred charge and its cargo
// count — and the footer hints ("Right-click: module menu", "Click:
// activate"). The returned key covers every input the model reads, so Sync
// only re-builds the panel when it would actually change — the builder is
// invoked through the row's Tip data exactly then (and on first attach),
// never as a per-frame throwaway.
func (r *Rack) cellTip(def *content.ModuleDef, slot sim.SlotRef) (string, func() TooltipModel) {
	// The rack-only inputs: which charge this cell prefers and how many of
	// it sit in cargo (the menu behind the hint shows the cycle action).
	charge, qty := "", 0
	if def.Weapon != nil && def.Weapon.AmmoClass != "" {
		id := r.ammoPref[slot]
		charge = r.ammoName(id)
		qty = r.cargoQty(id)
	}
	hints := []string{"Right-click: module menu"}
	if def.ActivationTime > 0 {
		hints = append(hints, "Click: activate")
	}
	key := string(def.ID) + "|" + charge + "|" + strconv.Itoa(qty)
	return key, func() TooltipModel {
		return moduleTip(def, slotLabel(slot), charge, qty, hints...)
	}
}

// cargoQty counts the player's cargo stacks of one item (0 when the sim or
// the cargo is gone).
func (r *Rack) cargoQty(id content.ItemID) int {
	s := r.cv
	if s == nil || id == "" {
		return 0
	}
	cg, ok := s.Cargo(s.Player())
	if !ok {
		return 0
	}
	qty := 0
	for _, st := range cg.Stacks {
		if st.Item == id {
			qty += st.Qty
		}
	}
	return qty
}

// ammoName renders a charge preference for the tooltip ("" = Auto).
func (r *Rack) ammoName(id content.ItemID) string {
	if id == "" {
		return "Auto"
	}
	if def, ok := r.cv.Defs().Item(id); ok {
		return def.Name
	}
	return string(id)
}

// ammoCandidates returns the charge choices for a weapon class: "" (Auto)
// first, then every distinct cargo item of that ammo class ordered by item
// id for determinism.
func (r *Rack) ammoCandidates(class string) []content.ItemID {
	var stacks []content.CargoStack
	if s := r.cv; s != nil {
		if cg, ok := s.Cargo(s.Player()); ok {
			stacks = cg.Stacks
		}
	}
	seen := map[content.ItemID]bool{}
	var ids []content.ItemID
	for _, st := range stacks {
		if st.Qty <= 0 || seen[st.Item] {
			continue
		}
		def, ok := r.cv.Defs().Item(st.Item)
		if !ok || def.AmmoClass != class {
			continue
		}
		seen[st.Item] = true
		ids = append(ids, st.Item)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]content.ItemID, 0, len(ids)+1)
	out = append(out, "")
	return append(out, ids...)
}

// toggleModule emits the activation toggle for one cell: Deactivate
// while the module runs, Activate otherwise. Empty slots are inert; the
// sim validates everything else (capacitor, locks, ranges) and logs its
// own refusals — the rack never mutates sim state.
func (r *Rack) toggleModule(cell *rackCell) {
	if cell.module == "" || r.as == nil {
		return
	}
	if cell.active {
		r.as.Do(Deactivate{Slot: cell.slot})
		return
	}
	def, _ := r.cv.Defs().Module(cell.module)
	r.as.Do(Activate{Slot: cell.slot, Target: r.clickTarget(def)})
}

// clickTarget resolves the activation target for a click the way EVE's
// "fire on the active target" works: the player's selected entity when it
// is fully locked and suits the module, else the first fully-locked target
// that suits — a ship for weapons, an asteroid for miners, any lock for
// ewar — and the zero entity (self) for everything else. Without a
// suitable lock the sim refuses with a logged warning.
func (r *Rack) clickTarget(def *content.ModuleDef) ecs.Entity {
	s := r.cv
	if s == nil || def == nil {
		return ecs.Entity(0)
	}
	tg, ok := s.Targeting(s.Player())
	if !ok {
		return ecs.Entity(0)
	}
	if r.pick != nil {
		if e := r.pick(); e != 0 && targetSuits(s, def, e) {
			for _, l := range tg.Locks {
				if l.Target == e && l.Progress >= 1 {
					return e
				}
			}
		}
	}
	for _, l := range tg.Locks {
		if l.Progress < 1 {
			continue
		}
		switch {
		case def.Weapon != nil:
			if _, ok := s.ShipState(l.Target); ok {
				return l.Target
			}
		case def.Mining != nil:
			if _, ok := s.Asteroid(l.Target); ok {
				return l.Target
			}
		case def.Kind == content.KindStasisWeb || def.Kind == content.KindWarpScram:
			return l.Target
		default:
			return ecs.Entity(0) // self-targeted modules take no target
		}
	}
	return ecs.Entity(0)
}

// targetSuits reports whether entity e is a valid target class for def
// (weapon→ship, mining→asteroid, ewar→anything, else self).
func targetSuits(s ContentView, def *content.ModuleDef, e ecs.Entity) bool {
	switch {
	case def.Weapon != nil:
		_, ok := s.ShipState(e)
		return ok
	case def.Mining != nil:
		_, ok := s.Asteroid(e)
		return ok
	case def.Kind == content.KindStasisWeb || def.Kind == content.KindWarpScram:
		return true
	default:
		return false
	}
}

// cycleNext is the next charge a "Cycle charge" press loads: the entry
// after pref in cands (wrapping), or the first entry when pref is not a
// candidate at all. ok is false when there is nothing to cycle to (a
// single candidate). The rack mirrors the result on dispatch (see
// NewRack) and the intent carries it, so the menu's press and the
// declared payload cannot disagree.
func cycleNext(cands []content.ItemID, pref content.ItemID) (content.ItemID, bool) {
	if len(cands) < 2 {
		return "", false
	}
	for i, id := range cands {
		if id == pref {
			return cands[(i+1)%len(cands)], true
		}
	}
	return cands[0], true
}

// pctLabel renders a fraction as a rounded percentage ("45%").
func pctLabel(f float64) string {
	return strconv.FormatInt(int64(math.Round(f*100)), 10) + "%"
}
