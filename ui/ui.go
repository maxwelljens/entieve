package ui

import (
	"image"
	"image/color"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Input is the per-frame input snapshot the game hands to Update. The game
// owns ebiten polling; the UI stays free of global input calls so it can be
// driven (and tested) with synthetic input.
type Input struct {
	MouseX, MouseY float64
	JustPressed    []ebiten.Key
	KeysDown       []ebiten.Key // held keys, for Shift/Ctrl modifiers
	MouseButton    [3]bool      // held: [0] left, [1] right, [2] middle
	JustClicked    [2]bool      // press edges: [0] left, [1] right
	Scroll         float64
	RealSec        float64 // real seconds since the previous frame (UI clocks)
}

// LogLine is one retained event for the combat/action log.
type LogLine struct {
	Time float64
	Text string
	Sev  sim.Severity
}

// maxLogLines caps the event log feed.
const maxLogLines = 200

// defaultFrameSec is the UI clock step assumed when the game reports no
// frame delta (also what synthetic test input gets).
const defaultFrameSec = 1.0 / 60.0

// marketQtySteps are the market/refine quantity stepper values.
var marketQtySteps = [...]int{1, 5, 10, 50, 100}

// Station service tabs.
const (
	tabRepair = iota
	tabMarket
	tabRefine
	tabFitting
	numStationTabs
)

// winDrag is an active title-bar drag (pointer grab offset from the
// window's top-left).
type winDrag struct {
	id     WinID
	dx, dy float64
}

// sbDragState is an active overview scrollbar-thumb drag. Scroll follows
// the pointer linearly from where the drag started (no feedback loop).
type sbDragState struct {
	y0, scroll0       float64
	travel, maxScroll float64
	contentH, viewH   float64
}

// UI is the EVE-style interface: windows, selection and input handling.
// It never mutates sim state; every player action becomes a sim.Command
// through the enqueue callback.
type UI struct {
	c       *content.Content
	s       *sim.Sim
	enqueue func(sim.Command)

	// Selected is the current selected-item entity.
	Selected ecs.Entity
	log      []LogLine

	screenW, screenH float64 // logical screen size, refreshed by Draw

	sortCol      SortColumn
	sortAsc      bool
	orbitIdx     int
	warpIdx      int // warp-to-within choice (index into WarpRanges)
	stationTab   int
	marketQtyIdx int
	mapSel       content.SystemID
	timeScale    int
	tabIdx       int // Tab cycle-lock position

	// Window management: per-session positions, open state and z-order.
	wins   map[WinID]*winState
	zorder []WinID
	drag   *winDrag
	sbDrag *sbDragState

	// Pointer/clock state read back during Draw for hover/pressed
	// feedback and tooltip timing.
	now        float64
	mx, my     float64
	mouse0     bool
	hoverRow   int // hovered module rack cell (-1 = none)
	hoverCell  int
	hoverSince float64

	// Overview state (per session).
	ovScroll     float64
	ovFilterOpen bool
	ovFilters    KindFilters

	// Scanner sweep state.
	scanT      float64
	scanCounts []ScanCount

	// Per-slot charge choices ("" = Auto), mirrored from CmdSetAmmo.
	ammoSel map[sim.SlotRef]content.ItemID
}

// New builds a UI over loaded content and sim. enqueue receives every player
// action as a command; the UI never touches sim state directly.
func New(c *content.Content, s *sim.Sim, enqueue func(sim.Command)) *UI {
	u := &UI{
		c:         c,
		s:         s,
		enqueue:   enqueue,
		sortCol:   SortDistance,
		sortAsc:   true,
		screenW:   1280,
		screenH:   720,
		timeScale: 1,
		tabIdx:    -1,
		hoverRow:  -1,
		hoverCell: -1,
		ovFilters: AllFilters(),
	}
	u.initWindows()
	return u
}

// Bind points the UI at a (re)built sim, e.g. after a mid-run F9 restore.
// It also clears selection (which may reference a dead entity) and the
// mirrored charge choices (sim ammo preferences reset on Restore).
func (u *UI) Bind(s *sim.Sim) {
	u.s = s
	u.Selected = 0
	u.tabIdx = -1
	u.ammoSel = nil
	u.scanCounts = nil
}

// SetTimeScale reports the current time acceleration (1/2/4/8) for the HUD.
func (u *UI) SetTimeScale(n int) { u.timeScale = n }

// Select sets the selected entity (used by world clicks in the game scene).
func (u *UI) Select(e ecs.Entity) { u.Selected = e }

// PushEvents appends sim events to the log buffer (capped at 200 lines).
// The game drains sim.Events() itself and hands them here.
func (u *UI) PushEvents(evs []sim.Event) {
	for _, e := range evs {
		if e.Text == "" {
			continue
		}
		u.log = append(u.log, LogLine{Time: e.Time, Text: e.Text, Sev: e.Severity})
	}
	if len(u.log) > maxLogLines {
		trim := len(u.log) - maxLogLines
		u.log = append(u.log[:0], u.log[trim:]...)
	}
}

// marketQty returns the current stepper quantity.
func (u *UI) marketQty() int { return marketQtySteps[u.marketQtyIdx%len(marketQtySteps)] }

// reg returns the layout regions for the current screen size.
func (u *UI) reg() Regions { return ComputeRegions(u.screenW, u.screenH) }

// Layout returns the current window regions (used by game scenes to place
// scene chrome, e.g. the station undock button).
func (u *UI) Layout() Regions { return u.reg() }

// docked reports whether the player is inside a station.
func (u *UI) docked() bool {
	if u.s == nil {
		return false
	}
	p := u.s.Player()
	if p.IsZero() {
		return false
	}
	return u.s.Docked.Has(p)
}

// topWindowAt returns the frontmost visible window containing the point
// (used to keep scroll/tooltips with the window the pointer is really on).
func (u *UI) topWindowAt(x, y float64) WinID {
	reg := u.reg()
	vis := u.visibleWindows()
	for i := len(vis) - 1; i >= 0; i-- {
		if u.WindowRect(vis[i], reg).Contains(x, y) {
			return vis[i]
		}
	}
	return ""
}

// PanelAt reports whether (x, y) is over any visible UI panel. The space
// scene uses it to leave panel clicks (and wheel zoom) to the UI.
func (u *UI) PanelAt(x, y float64) bool {
	reg := u.reg()
	for _, id := range u.visibleWindows() {
		if u.WindowRect(id, reg).Contains(x, y) {
			return true
		}
	}
	if u.docked() {
		return false
	}
	return reg.Targets.Contains(x, y) || reg.HUD.Contains(x, y)
}

// ---------------------------------------------------------------------------
// Update: keys, window chrome, clicks to commands
// ----------------------------------------------------------------:

// Update hit-tests the mouse, drags windows, scrolls the overview and maps
// keys to sim commands.
func (u *UI) Update(in Input) error {
	if u.s == nil {
		return nil
	}
	dt := in.RealSec
	if dt <= 0 || dt > 0.25 {
		dt = defaultFrameSec
	}
	u.now += dt
	u.mx, u.my = in.MouseX, in.MouseY
	u.mouse0 = in.MouseButton[0]

	// Ctrl-combos (Ctrl+S save) belong to the game layer; while Ctrl is
	// held the UI ignores keyboard input but still handles the mouse.
	ctrl := keyDown(in.KeysDown, ebiten.KeyControl)
	shift := keyDown(in.KeysDown, ebiten.KeyShift)
	for _, k := range in.JustPressed {
		if ctrl {
			break
		}
		switch k {
		case ebiten.KeyTab:
			if shift {
				u.lockSelected()
			} else {
				u.cycleLock()
			}
		case ebiten.KeyF1:
			u.toggleModule(content.SlotHigh, 0)
		case ebiten.KeyF2:
			u.toggleModule(content.SlotHigh, 1)
		case ebiten.KeyF3:
			u.toggleModule(content.SlotHigh, 2)
		case ebiten.KeyF4:
			u.toggleModule(content.SlotHigh, 3)
		case ebiten.KeyF5:
			u.toggleModule(content.SlotHigh, 4)
		case ebiten.KeyF6:
			u.toggleModule(content.SlotHigh, 5)
		case ebiten.KeyF7:
			u.toggleModule(content.SlotHigh, 6)
		case ebiten.KeyF8:
			u.toggleModule(content.SlotHigh, 7)
		case ebiten.KeyA:
			u.doAction(ActionApproach)
		case ebiten.KeyQ:
			u.doAction(ActionOrbit)
		case ebiten.KeyS:
			u.enqueue(sim.CmdStop{})
		case ebiten.KeyW:
			u.doAction(ActionAlign)
		case ebiten.KeyZ:
			u.doAction(ActionWarp)
		case ebiten.KeyD:
			u.dockNearest()
		case ebiten.KeyJ:
			u.jumpNearest()
		case ebiten.KeyL:
			u.lootNearest()
		case ebiten.KeyM:
			u.mineSelected()
		case ebiten.KeyI:
			u.ToggleWindow(WinInventory)
		case ebiten.KeyF:
			u.ToggleWindow(WinFitting)
		case ebiten.KeyC:
			u.ToggleWindow(WinScanner)
		case ebiten.KeyG:
			if u.docked() {
				u.stationTab = tabMarket
			}
		}
	}

	if in.Scroll != 0 {
		u.handleScroll(in)
	}
	u.dragUpdate(in)
	if in.JustClicked[0] {
		u.click(in)
	}
	if in.JustClicked[1] {
		u.rightClickAmmo(in)
	}
	u.updateModuleHover(in)
	if u.WindowOpen(WinScanner) {
		u.advanceScanner(dt)
	}
	return nil
}

// keyDown reports whether k is in held.
func keyDown(held []ebiten.Key, k ebiten.Key) bool {
	for _, h := range held {
		if h == k {
			return true
		}
	}
	return false
}

// handleScroll scrolls the overview when the wheel turns over it.
func (u *UI) handleScroll(in Input) {
	if u.docked() || u.drag != nil || u.sbDrag != nil {
		return
	}
	if u.topWindowAt(in.MouseX, in.MouseY) != WinOverview {
		return
	}
	r := u.WindowRect(WinOverview, u.reg())
	body := OverviewBody(r)
	if !body.Contains(in.MouseX, in.MouseY) && !OverviewHeaderRect(r).Contains(in.MouseX, in.MouseY) {
		return
	}
	rows := u.overviewRows()
	contentH := float64(len(rows)) * ovRowH
	// Wheel up (+1) moves earlier rows into view (offset shrinks).
	u.ovScroll = ClampScroll(u.ovScroll-in.Scroll*ovRowH, contentH, body.H)
}

// dragUpdate continues an active title-bar or scrollbar drag.
func (u *UI) dragUpdate(in Input) {
	if u.drag != nil {
		if !in.MouseButton[0] {
			u.drag = nil
		} else {
			id := u.drag.id
			reg := u.reg()
			st := u.winStateOf(id)
			st.rect = DragRect(u.WindowRect(id, reg), u.drag.dx, u.drag.dy,
				in.MouseX, in.MouseY, u.screenW, u.screenH)
			st.moved = true
		}
	}
	if u.sbDrag != nil {
		if !in.MouseButton[0] {
			u.sbDrag = nil
		} else if u.sbDrag.travel > 0 {
			d := u.sbDrag
			delta := (in.MouseY - d.y0) * d.maxScroll / d.travel
			u.ovScroll = ClampScroll(d.scroll0+delta, d.contentH, d.viewH)
		}
	}
}

// click dispatches a left click to the topmost window's chrome or content.
func (u *UI) click(in Input) {
	reg := u.reg()
	// The overview's filter popup dismisses on any press outside it.
	if u.ovFilterOpen {
		r := u.WindowRect(WinOverview, reg)
		if !OverviewFilterPopupRect(r).Contains(in.MouseX, in.MouseY) &&
			!OverviewFilterButtonRect(r).Contains(in.MouseX, in.MouseY) {
			u.ovFilterOpen = false
		}
	}
	vis := u.visibleWindows()
	for i := len(vis) - 1; i >= 0; i-- {
		id := vis[i]
		r := u.WindowRect(id, reg)
		if !r.Contains(in.MouseX, in.MouseY) {
			continue
		}
		switch u.chromeAt(id, r, in.MouseX, in.MouseY) {
		case chromeClose:
			u.SetWindowOpen(id, false)
			return
		case chromeFilter:
			u.ovFilterOpen = !u.ovFilterOpen
			return
		case chromeGrab:
			dx, dy, _ := TitleGrabOffset(r, in.MouseX, in.MouseY)
			u.raise(id)
			u.drag = &winDrag{id: id, dx: dx, dy: dy}
			return
		}
		handled := u.dispatchWindow(id, in.MouseX, in.MouseY, reg, r)
		if handled || !clickFallsThrough(id) {
			u.raise(id)
			return
		}
		// Contentless fall-through (log over the station panel).
	}
	// Chrome panels sit beneath every window (space scene only).
	if u.docked() {
		return
	}
	if reg.Targets.Contains(in.MouseX, in.MouseY) {
		u.clickTargets(in.MouseX, in.MouseY, reg)
	}
}

// dispatchWindow routes a content click to the window's handler. It
// reports whether the window consumed the click.
func (u *UI) dispatchWindow(id WinID, x, y float64, reg Regions, r Rect) bool {
	switch id {
	case WinOverview:
		u.clickOverview(x, y, r)
		return true
	case WinSelected:
		u.clickSelected(x, y, r)
		return true
	case WinModules:
		u.clickModules(x, y, r)
		return true
	case WinStation:
		u.clickStation(x, y, r)
		return true
	case WinFitting:
		u.clickFitting(x, y, r)
		return true
	}
	return false
}

// clickOverview handles the filter popup, header sorting, scrollbar
// dragging and row selection.
func (u *UI) clickOverview(x, y float64, r Rect) {
	if u.ovFilterOpen {
		p := OverviewFilterPopupRect(r)
		for i := range filterGroups {
			if OverviewFilterRowRect(p, i).Contains(x, y) {
				u.ovFilters.Toggle(filterGroups[i])
				return
			}
		}
		if p.Contains(x, y) {
			return
		}
		u.ovFilterOpen = false // outside the popup: close and continue
	}
	// Column header strip: sort (click again flips direction).
	if OverviewHeaderRect(r).Contains(x, y) {
		if col := OverviewColumnAt(r, x); col >= 0 {
			sc := SortColumn(col)
			if u.sortCol == sc {
				u.sortAsc = !u.sortAsc
			} else {
				u.sortCol, u.sortAsc = sc, true
			}
		}
		return
	}
	rows := u.overviewRows()
	body := OverviewBody(r)
	contentH := float64(len(rows)) * ovRowH
	viewH := body.H
	if ScrollMax(contentH, viewH) > 0 {
		thumb := ScrollThumbRect(ScrollbarTrack(body), contentH, viewH, u.ovScroll)
		if thumb.Contains(x, y) {
			track := ScrollbarTrack(body)
			u.sbDrag = &sbDragState{
				y0: y, scroll0: u.ovScroll,
				travel:    track.H - thumb.H,
				maxScroll: ScrollMax(contentH, viewH),
				contentH:  contentH, viewH: viewH,
			}
			return
		}
	}
	if row := OverviewRowAt(r, y, u.ovScroll, len(rows)); row >= 0 {
		u.Selected = rows[row].Entity
	}
}

// clickSelected runs the warp-range control and action buttons.
func (u *UI) clickSelected(x, y float64, r Rect) {
	// Warp-to-within segmented control (a preference: selectable even
	// without a target).
	for i, sr := range WarpSegRects(r, len(WarpRanges)) {
		if sr.Contains(x, y) {
			u.warpIdx = CycleWarpRange(i)
			return
		}
	}
	if WarpButtonRect(r, len(WarpRanges)).Contains(x, y) {
		u.doAction(ActionWarp)
		return
	}
	info, ok := u.s.Info(u.Selected)
	if !ok {
		return
	}
	grid := actionsExcept(ActionsFor(info), ActionWarp)
	for i, ar := range ActionRects(r, len(grid)) {
		if ar.Contains(x, y) {
			u.doAction(grid[i])
			return
		}
	}
}

// clickTargets selects the target box under the cursor (chrome panel).
func (u *UI) clickTargets(x, y float64, reg Regions) {
	locks := u.locks()
	for i, br := range TargetBoxRects(reg.Targets, len(locks)) {
		if br.Contains(x, y) {
			u.Selected = locks[i].Target
			return
		}
	}
}

// clickModules toggles the module cell under the cursor.
func (u *UI) clickModules(x, y float64, r Rect) {
	row, i := ModuleCellAt(r, x, y)
	if row < 0 {
		return
	}
	u.toggleModule(slotClassOf(row), i)
}

// slotClassOf maps a rack row to its slot class.
func slotClassOf(row int) string {
	switch row {
	case 0:
		return content.SlotHigh
	case 1:
		return content.SlotMid
	default:
		return content.SlotLow
	}
}

// rightClickAmmo cycles the loaded charge of a weapon cell (right-click):
// Auto → each cargo charge of the weapon's ammo class → Auto. Non-weapons
// and weapons with no matching cargo are a clean no-op.
func (u *UI) rightClickAmmo(in Input) {
	if u.docked() {
		return
	}
	if u.topWindowAt(in.MouseX, in.MouseY) != WinModules {
		return // occluded by another window: not our cell
	}
	r := u.WindowRect(WinModules, u.reg())
	if !r.Contains(in.MouseX, in.MouseY) {
		return
	}
	row, i := ModuleCellAt(r, in.MouseX, in.MouseY)
	if row < 0 {
		return
	}
	class := slotClassOf(row)
	fit, ok := u.s.Fittings.Get(u.s.Player())
	if !ok {
		return
	}
	defs := fit.Defs(class)
	if i < 0 || i >= len(defs) {
		return
	}
	def, ok := u.c.Module(defs[i])
	if !ok || def.Kind != content.KindWeapon || def.Weapon == nil || def.Weapon.AmmoClass == "" {
		return // disabled cleanly for non-weapons
	}
	cargo, ok := u.s.Cargos.Get(u.s.Player())
	if !ok {
		return
	}
	cands := AmmoCandidates(u.c, cargo.Stacks, def.Weapon.AmmoClass)
	if len(cands) == 0 {
		return
	}
	slot := sim.SlotRef{Class: class, Index: i}
	next := NextAmmoChoice(u.ammoSel[slot], cands)
	if u.ammoSel == nil {
		u.ammoSel = map[sim.SlotRef]content.ItemID{}
	}
	u.ammoSel[slot] = next
	u.enqueue(sim.CmdSetAmmo{Slot: slot, Item: next})
}

// updateModuleHover tracks the hovered rack cell for tooltips.
func (u *UI) updateModuleHover(in Input) {
	prevRow, prevCell := u.hoverRow, u.hoverCell
	u.hoverRow, u.hoverCell = -1, -1
	if !u.docked() {
		r := u.WindowRect(WinModules, u.reg())
		if r.Contains(in.MouseX, in.MouseY) {
			if row, cell := ModuleCellAt(r, in.MouseX, in.MouseY); row >= 0 {
				u.hoverRow, u.hoverCell = row, cell
			}
		}
	}
	if u.hoverRow != prevRow || u.hoverCell != prevCell {
		u.hoverSince = u.now
	}
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// overviewRows returns the sorted, filtered rows backing the overview:
// presentation-only entities (Info.Visible: landmarks + sensor range)
// minus the kind filters the player has toggled off.
func (u *UI) overviewRows() []sim.Info {
	rows := make([]sim.Info, 0, 32)
	for _, inf := range u.presentationGrid() {
		if u.ovFilters.Passes(inf.Kind) {
			rows = append(rows, inf)
		}
	}
	SortOverview(rows, u.sortCol, u.sortAsc)
	return rows
}

// locks returns the player's target locks (nil if none).
func (u *UI) locks() []sim.Lock {
	if u.s == nil {
		return nil
	}
	tg, ok := u.s.Targetings.Get(u.s.Player())
	if !ok {
		return nil
	}
	return tg.Locks
}

// actionsExcept returns acts without the given action (the warp button
// lives in its own row next to the range segments).
func actionsExcept(acts []Action, skip Action) []Action {
	out := make([]Action, 0, len(acts))
	for _, a := range acts {
		if a != skip {
			out = append(out, a)
		}
	}
	return out
}

// doAction maps a contextual action to its sim command.
func (u *UI) doAction(a Action) {
	if u.s == nil || u.Selected.IsZero() {
		return
	}
	info, ok := u.s.Info(u.Selected)
	if !ok {
		u.Selected = 0
		return
	}
	switch a {
	case ActionApproach:
		u.enqueue(sim.CmdApproach{Target: u.Selected})
	case ActionOrbit:
		rng := OrbitRanges[u.orbitIdx%len(OrbitRanges)]
		u.orbitIdx = (u.orbitIdx + 1) % len(OrbitRanges)
		u.enqueue(sim.CmdOrbit{Target: u.Selected, Range: rng})
	case ActionAlign:
		u.enqueue(sim.CmdAlignTo{Target: u.Selected})
	case ActionWarp:
		if info.CanWarp {
			u.enqueue(sim.CmdWarpTo{Target: u.Selected, Range: WarpRangeMeters(u.warpIdx)})
		}
	case ActionLock:
		u.enqueue(sim.CmdLock{Target: u.Selected})
	case ActionMine:
		u.mineSelected()
	case ActionLoot:
		if info.CanLoot {
			u.enqueue(sim.CmdLoot{Wreck: u.Selected})
		}
	case ActionDock:
		if info.CanDock {
			u.enqueue(sim.CmdDock{Station: u.Selected})
		}
	case ActionJump:
		if info.CanJump {
			u.enqueue(sim.CmdJump{Gate: u.Selected})
		}
	}
}

// nearest returns the closest perceivable grid entity satisfying match.
func (u *UI) nearest(match func(sim.Info) bool) (sim.Info, bool) {
	var best sim.Info
	found := false
	for _, inf := range u.presentationGrid() {
		if !match(inf) {
			continue
		}
		if !found || inf.Distance < best.Distance {
			best, found = inf, true
		}
	}
	return best, found
}

// dockNearest docks at the selected station, else the nearest dockable one.
func (u *UI) dockNearest() {
	if !u.Selected.IsZero() {
		if info, ok := u.s.Info(u.Selected); ok && info.CanDock {
			u.enqueue(sim.CmdDock{Station: u.Selected})
			return
		}
	}
	if inf, ok := u.nearest(func(i sim.Info) bool { return i.CanDock }); ok {
		u.enqueue(sim.CmdDock{Station: inf.Entity})
	}
}

// jumpNearest jumps through the selected gate, else the nearest jumpable one.
func (u *UI) jumpNearest() {
	if !u.Selected.IsZero() {
		if info, ok := u.s.Info(u.Selected); ok && info.CanJump {
			u.enqueue(sim.CmdJump{Gate: u.Selected})
			return
		}
	}
	if inf, ok := u.nearest(func(i sim.Info) bool { return i.CanJump }); ok {
		u.enqueue(sim.CmdJump{Gate: inf.Entity})
	}
}

// lootNearest loots the closest wreck.
func (u *UI) lootNearest() {
	if inf, ok := u.nearest(func(i sim.Info) bool { return i.CanLoot }); ok {
		u.enqueue(sim.CmdLoot{Wreck: inf.Entity})
	}
}

// mineSelected fires the first mining laser at the selected asteroid.
func (u *UI) mineSelected() {
	if u.s == nil || u.Selected.IsZero() {
		return
	}
	info, ok := u.s.Info(u.Selected)
	if !ok || info.Kind != sim.KindAsteroid {
		return
	}
	slot, ok := u.firstMiningSlot()
	if !ok {
		return
	}
	u.enqueue(sim.CmdMine{Asteroid: u.Selected, Slot: slot})
}

// firstMiningSlot finds the first fitted mining laser.
func (u *UI) firstMiningSlot() (sim.SlotRef, bool) {
	fit, ok := u.s.Fittings.Get(u.s.Player())
	if !ok {
		return sim.SlotRef{}, false
	}
	for _, class := range []string{content.SlotHigh, content.SlotMid} {
		for i, id := range fit.Defs(class) {
			if def, ok := u.c.Module(id); ok && def.Kind == content.KindMiningLaser {
				return sim.SlotRef{Class: class, Index: i}, true
			}
		}
	}
	return sim.SlotRef{}, false
}

// toggleModule activates/deactivates a fitted module by slot class and index.
// The target is the current selection (zero = self, per sim contract).
func (u *UI) toggleModule(class string, idx int) {
	fit, ok := u.s.Fittings.Get(u.s.Player())
	if !ok {
		return
	}
	rts := fit.Runtimes(class)
	if idx < 0 || idx >= len(rts) {
		return
	}
	slot := sim.SlotRef{Class: class, Index: idx}
	if rts[idx].Active {
		u.enqueue(sim.CmdDeactivate{Slot: slot})
		return
	}
	u.enqueue(sim.CmdActivate{Slot: slot, Target: u.Selected})
}

// cycleLock locks the next-nearest unselected ship (Tab): distance-ordered
// cycling over every perceivable lockable ship on grid.
func (u *UI) cycleLock() {
	var ships []sim.Info
	player := u.s.Player()
	for _, inf := range u.presentationGrid() {
		if canLock(inf.Kind) && inf.Entity != player {
			ships = append(ships, inf)
		}
	}
	if len(ships) == 0 {
		return
	}
	SortOverview(ships, SortDistance, true)
	u.tabIdx++
	if u.tabIdx < 0 || u.tabIdx >= len(ships) {
		u.tabIdx = 0
	}
	u.Selected = ships[u.tabIdx].Entity
	u.enqueue(sim.CmdLock{Target: ships[u.tabIdx].Entity})
}

// lockSelected locks the current selection (Shift+Tab).
func (u *UI) lockSelected() {
	if u.Selected.IsZero() {
		return
	}
	u.enqueue(sim.CmdLock{Target: u.Selected})
}

// ---------------------------------------------------------------------------
// Draw
// ---------------------------------------------------------------------------

// Draw renders all visible windows. It reads sim state read-only.
func (u *UI) Draw(dst *ebiten.Image, cam render.Camera, s *sim.Sim) {
	if s != nil {
		u.s = s
	}
	if cam.ScreenW > 0 && cam.ScreenH > 0 {
		u.screenW, u.screenH = cam.ScreenW, cam.ScreenH
	} else {
		b := dst.Bounds()
		u.screenW, u.screenH = float64(b.Dx()), float64(b.Dy())
	}
	reg := u.reg()
	if u.docked() {
		// Veil the world behind the station services panel.
		vector.DrawFilledRect(dst, 0, 0, float32(u.screenW), float32(u.screenH),
			color.RGBA{0, 0, 0, 150}, false)
	} else {
		u.drawHUD(dst, s, reg) // chrome: HUD + target boxes
	}
	for _, id := range u.visibleWindows() {
		switch id {
		case WinStation:
			u.drawStation(dst, s, reg)
		case WinOverview:
			u.drawOverview(dst, s, reg)
		case WinSelected:
			u.drawSelected(dst, s, reg)
		case WinLog:
			u.drawLog(dst, reg)
		case WinModules:
			u.drawModules(dst, s, reg)
		case WinScanner:
			u.drawScanner(dst, reg)
		case WinInventory:
			u.drawInventory(dst, s, reg)
		case WinFitting:
			u.drawFitting(dst, s, reg)
		case WinStarmap, WinHelp:
			// Drawn by their scenes over the whole stack.
		}
	}
	u.drawModuleTooltip(dst, reg)
}

// drawModuleTooltip shows the hovered module cell's tooltip after the
// 300 ms hover delay, above every window.
func (u *UI) drawModuleTooltip(dst *ebiten.Image, reg Regions) {
	if u.hoverRow < 0 || u.hoverCell < 0 {
		return
	}
	if !TooltipVisible(u.hoverSince, u.now) {
		return
	}
	if u.topWindowAt(u.mx, u.my) != WinModules {
		return
	}
	lines := u.moduleTipLines(u.hoverRow, u.hoverCell)
	if len(lines) == 0 {
		return
	}
	r := u.WindowRect(WinModules, reg)
	cell := ModuleCellRect(r, u.hoverRow, u.hoverCell)
	DrawTooltip(dst, TooltipRect(cell.Right(), cell.Y, u.screenW, u.screenH, lines), lines)
}

// moduleTipLines builds the tooltip for a rack cell (nil when empty).
func (u *UI) moduleTipLines(row, cell int) []string {
	if u.s == nil || u.c == nil {
		return nil
	}
	class := slotClassOf(row)
	fit, ok := u.s.Fittings.Get(u.s.Player())
	if !ok {
		return nil
	}
	defs := fit.Defs(class)
	if cell < 0 || cell >= len(defs) {
		return nil
	}
	def, ok := u.c.Module(defs[cell])
	if !ok {
		return nil
	}
	slotLabel := [...]string{"High", "Mid", "Low"}[row] + " slot " + itoa(cell+1)
	slot := sim.SlotRef{Class: class, Index: cell}
	ammo := AmmoChoiceName(u.c, u.ammoSel[slot])
	return ModuleTipLines(def, slotLabel, ammo)
}

// subImage clips dst to r for scrollable content (row overflow must not
// paint over panel chrome). Falls back to dst if clipping is unavailable.
func subImage(dst *ebiten.Image, r Rect) *ebiten.Image {
	ir := image.Rect(int(r.X), int(r.Y), int(r.Right())+1, int(r.Bottom())+1)
	if ir.Empty() {
		return dst
	}
	sub := dst.SubImage(ir)
	if sub == nil {
		return dst
	}
	if img, ok := sub.(*ebiten.Image); ok {
		return img
	}
	return dst
}
