package uikit

import (
	"image"
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
	rackNameH    = 16                     // name line under a tile (fits StepTiny)
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
// the module (enqueueing CmdActivate/CmdDeactivate; empty slots are inert)
// and a second click inside the double-click window additionally opens Show
// Info. Right-clicking a cell opens its context menu — Show Info, the
// loaded charge as a second ref, and Cycle charge, which walks the old
// direct right-click's CmdSetAmmo step (Auto → the weapon's ammo-class
// charges sitting in cargo → Auto).
type Rack struct {
	env       Env
	container *widget.Container
	cells     [rackRows][rackMaxCells]*rackCell
	ammoPref  map[sim.SlotRef]content.ItemID
	measure   func(string) float64
	// pick, when set (game wires Kit.Selected), names the entity the player
	// currently has selected so click-activation targets it — the same
	// source the F-key module toggles use — before falling back to the
	// first suitable lock.
	pick func() ecs.Entity

	// double-click bookkeeping (second click on a cell shows info)
	lastCell *rackCell
	lastAt   time.Time
}

// rackCell is one slot: a cell column (icon tile + name line + cycle bar)
// plus its tooltip widgets. Cells are persistent, so a fitting change only
// rewrites their name, glyph, tooltip and enabled state. active mirrors
// ModuleRuntime.Active for the click toggle (the visual rides the button's
// Disabled state: dim cell while off, amber cell while running).
type rackCell struct {
	slot   sim.SlotRef
	root   *widget.Container
	btn    *widget.Button
	name   *widget.Text
	bar    *widget.ProgressBar
	tip    rowTip           // structured hover tooltip (built on first fit)
	module content.ModuleID // module currently shown ("" = empty slot)
	icon   string           // glyph the tile currently shows ("" = none)
	active bool             // runtime Active at the last Sync
}

// NewRack builds the module rack over the given environment. Its container
// anchors bottom-right in an anchor-layout root; Sync refreshes every cell
// from the player's fitting.
func NewRack(env Env) *Rack {
	p := env.Pal
	faceTiny := env.Face(StepTiny)
	r := &Rack{env: env, ammoPref: map[sim.SlotRef]content.ItemID{}}
	r.measure = func(s string) float64 {
		w, _ := text.Measure(s, *faceTiny, 0)
		return w
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(rackRowGap),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	for row := 0; row < rackRows; row++ {
		label := widget.NewText(
			widget.TextOpts.Text(rackRowLabels[row], faceTiny, solid(p.Head)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(rackLabelW, 0)),
		)
		rowBox := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(contentsPad),
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
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	)
	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, titleBarH),
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
			Padding:            &widget.Insets{Right: 10, Bottom: 10},
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
	p := r.env.Pal
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
		widget.ButtonOpts.GraphicPadding(widget.Insets{Left: 0, Right: rackIconPad}),
		widget.ButtonOpts.TextPadding(&widget.Insets{Left: 3, Right: 3, Top: 2, Bottom: 2}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(rackCellW, rackCellH),
			widget.WidgetOpts.MouseButtonClickedHandler(func(a *widget.WidgetMouseButtonClickedEventArgs) {
				switch a.Button {
				case ebiten.MouseButtonLeft:
					r.cellClicked(cell)
				case ebiten.MouseButtonRight:
					r.cellMenu(cell, ClickPoint(a))
				}
			}),
		),
	)
	// The name line under the tile: the same click seam as the tile, so
	// the whole cell stays clickable.
	cell.name = widget.NewText(
		widget.TextOpts.Text("", r.env.Face(StepTiny), solid(p.Dim)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, rackNameH),
			widget.WidgetOpts.MouseButtonClickedHandler(func(a *widget.WidgetMouseButtonClickedEventArgs) {
				switch a.Button {
				case ebiten.MouseButtonLeft:
					r.cellClicked(cell)
				case ebiten.MouseButtonRight:
					r.cellMenu(cell, ClickPoint(a))
				}
			}),
		),
	)
	cell.bar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, rackCycleMax, 0),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, rackCycleH)),
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
	if name == "" {
		return icons.Get(icons.Fallback, IconButton, color.RGBA{})
	}
	return icons.Get(name, IconButton, tint)
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
	s := r.env.Sim
	if s == nil {
		return
	}
	player := s.Player()
	fit, hasFit := s.Fittings.Get(player)
	now := s.Time()

	// Slot counts: the hull's slots (empty cells still show), capped at the
	// cells the rack was built with — mirrors ui's ModuleMaxCells clamp.
	var slots content.Slots
	if sh, ok := s.Ships.Get(player); ok {
		if def, ok := r.env.Content.Ship(sh.Hull); ok {
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
		def, _ = r.env.Content.Module(id)
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
			Idle:     rackGlyph(icon, r.env.Pal.Text),
			Disabled: rackGlyph(icon, iconColor(r.env.Pal.Dim, IconDisabledA)),
		})
	}
	cell.name.Label = r.shortName(def)
	nameCol := r.env.Pal.Dim
	if active {
		nameCol = r.env.Pal.Text
	}
	cell.name.SetColor(nameCol)
	if def == nil {
		cell.tip.set(r.env, cell.btn.GetWidget(), "", nil)
	} else {
		key, build := r.cellTip(def, cell.slot)
		cell.tip.set(r.env, cell.btn.GetWidget(), key, build)
	}
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
	return Ellipsize(name, rackCellW-6, r.measure)
}

// cellTip builds one fitted cell's tooltip: the shared module model plus
// what the rack alone knows — the slot, the preferred charge and its cargo
// count — and the footer hints ("Right-click: module menu", "Click:
// activate"). The returned key covers every input the model reads, so Sync
// only re-builds the panel when it would actually change — the builder is
// invoked by rowTip.set exactly then (and on first attach), never as a
// per-frame throwaway.
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
		return r.env.moduleTip(def, slotLabel(slot), charge, qty, hints...)
	}
}

// cargoQty counts the player's cargo stacks of one item (0 when the sim or
// the cargo is gone).
func (r *Rack) cargoQty(id content.ItemID) int {
	s := r.env.Sim
	if s == nil || id == "" {
		return 0
	}
	cg, ok := s.Cargos.Get(s.Player())
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
	if def, ok := r.env.Content.Item(id); ok {
		return def.Name
	}
	return string(id)
}

// ammoCandidates returns the charge choices for a weapon class: "" (Auto)
// first, then every distinct cargo item of that ammo class ordered by item
// id for determinism.
func (r *Rack) ammoCandidates(class string) []content.ItemID {
	var stacks []content.CargoStack
	if s := r.env.Sim; s != nil {
		if cg, ok := s.Cargos.Get(s.Player()); ok {
			stacks = cg.Stacks
		}
	}
	seen := map[content.ItemID]bool{}
	var ids []content.ItemID
	for _, st := range stacks {
		if st.Qty <= 0 || seen[st.Item] {
			continue
		}
		def, ok := r.env.Content.Item(st.Item)
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

// cellClicked handles a cell's left click. The click toggles — that is
// this surface's documented action ("Click: activate" in the tooltip),
// so every click fires it — and a repeat inside the double-click window
// additionally opens Show Info, mirroring the tables' activate-on-repeat
// pattern (the second click is never swallowed).
func (r *Rack) cellClicked(cell *rackCell) {
	r.toggleModule(cell)
	now := time.Now()
	if r.lastCell == cell && now.Sub(r.lastAt) < doubleClickWindow {
		r.lastCell, r.lastAt = nil, time.Time{} // the gesture is spent
		r.showCellInfo(cell)
		return
	}
	r.lastCell, r.lastAt = cell, now
}

// showCellInfo opens the cell's module in the Show Info window.
func (r *Rack) showCellInfo(cell *rackCell) {
	if cell.module == "" {
		return
	}
	r.env.ShowInfo(ModuleRef(cell.module))
}

// cellMenu opens the cell's context menu at the click position: Show Info
// first (bold, per the menu convention), the loaded charge as a second
// ref when one is preferred, then Cycle charge — the same CmdSetAmmo walk
// the direct right-click used to do, gated exactly where it could no-op.
func (r *Rack) cellMenu(cell *rackCell, at image.Point) {
	if cell.module == "" {
		return // empty slot: nothing to show or cycle
	}
	def, _ := r.env.Content.Module(cell.module)
	entries := []MenuEntry{
		{Label: "Show Info", Bold: true, Action: func() { r.showCellInfo(cell) }},
	}
	if id := r.ammoPref[cell.slot]; id != "" {
		entries = append(entries, MenuEntry{
			Label:  "Show Info — " + r.ammoName(id),
			Action: func() { r.env.ShowInfo(ItemRef(id)) },
		})
	}
	if def != nil && def.Weapon != nil && def.Weapon.AmmoClass != "" {
		cycleable := len(r.ammoCandidates(def.Weapon.AmmoClass)) > 1 && r.env.Enqueue != nil
		entries = append(entries, MenuSep, MenuEntry{
			Label:    "Cycle charge",
			Disabled: !cycleable,
			Action:   func() { r.cycleAmmo(cell) },
		})
	}
	r.env.OpenMenu(entries, at)
}

// toggleModule enqueues the activation toggle for one cell: CmdDeactivate
// while the module runs, CmdActivate otherwise. Empty slots are inert; the
// sim validates everything else (capacitor, locks, ranges) and logs its
// own refusals — the rack never mutates sim state.
func (r *Rack) toggleModule(cell *rackCell) {
	if cell.module == "" || r.env.Enqueue == nil {
		return
	}
	if cell.active {
		r.env.Enqueue(sim.CmdDeactivate{Slot: cell.slot})
		return
	}
	def, _ := r.env.Content.Module(cell.module)
	r.env.Enqueue(sim.CmdActivate{Slot: cell.slot, Target: r.clickTarget(def)})
}

// clickTarget resolves the activation target for a click the way EVE's
// "fire on the active target" works: the player's selected entity when it
// is fully locked and suits the module, else the first fully-locked target
// that suits — a ship for weapons, an asteroid for miners, any lock for
// ewar — and the zero entity (self) for everything else. Without a
// suitable lock the sim refuses with a logged warning.
func (r *Rack) clickTarget(def *content.ModuleDef) ecs.Entity {
	s := r.env.Sim
	if s == nil || def == nil {
		return ecs.Entity(0)
	}
	tg, ok := s.Targetings.Get(s.Player())
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
			if s.Ships.Has(l.Target) {
				return l.Target
			}
		case def.Mining != nil:
			if s.Asteroids.Has(l.Target) {
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
func targetSuits(s *sim.Sim, def *content.ModuleDef, e ecs.Entity) bool {
	switch {
	case def.Weapon != nil:
		return s.Ships.Has(e)
	case def.Mining != nil:
		return s.Asteroids.Has(e)
	case def.Kind == content.KindStasisWeb || def.Kind == content.KindWarpScram:
		return true
	default:
		return false
	}
}

// cycleAmmo advances a weapon cell's charge preference one step and enqueues
// CmdSetAmmo. Non-weapons, weapons without an ammo class and weapons with no
// matching cargo charge are left alone.
func (r *Rack) cycleAmmo(cell *rackCell) {
	if cell.module == "" || r.env.Enqueue == nil {
		return
	}
	def, ok := r.env.Content.Module(cell.module)
	if !ok || def.Weapon == nil || def.Weapon.AmmoClass == "" {
		return
	}
	cands := r.ammoCandidates(def.Weapon.AmmoClass)
	if len(cands) < 2 { // only Auto: nothing to cycle to
		return
	}
	next := cands[0]
	for i, id := range cands {
		if id == r.ammoPref[cell.slot] {
			next = cands[(i+1)%len(cands)]
			break
		}
	}
	if next == r.ammoPref[cell.slot] {
		return
	}
	r.ammoPref[cell.slot] = next
	r.env.Enqueue(sim.CmdSetAmmo{Slot: cell.slot, Item: next})
}

// pctLabel renders a fraction as a rounded percentage ("45%").
func pctLabel(f float64) string {
	return strconv.FormatInt(int64(math.Round(f*100)), 10) + "%"
}
