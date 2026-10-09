package ui

import (
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
}

// LogLine is one retained event for the combat/action log.
type LogLine struct {
	Time float64
	Text string
	Sev  sim.Severity
}

// maxLogLines caps the event log feed.
const maxLogLines = 200

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
	showInv      bool
	showFit      bool
	stationTab   int
	marketQtyIdx int
	mapSel       content.SystemID
	timeScale    int
	tabIdx       int // Tab cycle-lock position
}

// New builds a UI over loaded content and sim. enqueue receives every player
// action as a command; the UI never touches sim state directly.
func New(c *content.Content, s *sim.Sim, enqueue func(sim.Command)) *UI {
	return &UI{
		c:         c,
		s:         s,
		enqueue:   enqueue,
		sortCol:   SortDistance,
		sortAsc:   true,
		screenW:   1280,
		screenH:   720,
		timeScale: 1,
		tabIdx:    -1,
	}
}

// Bind points the UI at a (re)built sim, e.g. after a mid-run F9 restore.
// It also clears selection, which may reference a dead entity.
func (u *UI) Bind(s *sim.Sim) {
	u.s = s
	u.Selected = 0
	u.tabIdx = -1
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

// PanelAt reports whether (x, y) is over any visible UI panel. The space
// scene uses it to leave panel clicks to the UI.
func (u *UI) PanelAt(x, y float64) bool {
	reg := u.reg()
	if u.docked() {
		return reg.Station.Contains(x, y) ||
			(u.showInv && reg.Inventory.Contains(x, y)) ||
			(u.showFit && reg.Fitting.Contains(x, y))
	}
	return reg.Overview.Contains(x, y) || reg.Selected.Contains(x, y) ||
		reg.Targets.Contains(x, y) || reg.HUD.Contains(x, y) ||
		reg.Modules.Contains(x, y) || reg.Log.Contains(x, y) ||
		(u.showInv && reg.Inventory.Contains(x, y)) ||
		(u.showFit && reg.Fitting.Contains(x, y))
}

// ---------------------------------------------------------------------------
// Update: keys and clicks to commands
// ---------------------------------------------------------------------------

// Update hit-tests the mouse and maps keys to sim commands.
func (u *UI) Update(in Input) error {
	if u.s == nil {
		return nil
	}
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
			u.showInv = !u.showInv
		case ebiten.KeyF:
			u.showFit = !u.showFit
		case ebiten.KeyG:
			if u.docked() {
				u.stationTab = tabMarket
			}
		}
	}
	if in.JustClicked[0] {
		u.click(in.MouseX, in.MouseY)
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

// click dispatches a left click to the topmost panel under the cursor.
func (u *UI) click(x, y float64) {
	reg := u.reg()
	// Overlays first: they sit above the base HUD.
	if u.showFit && reg.Fitting.Contains(x, y) {
		u.clickFitting(x, y, reg)
		return
	}
	if u.showInv && reg.Inventory.Contains(x, y) {
		return // list only, no interactive rows yet
	}
	if u.docked() {
		if reg.Station.Contains(x, y) {
			u.clickStation(x, y, reg)
		}
		return
	}
	switch {
	case reg.Overview.Contains(x, y):
		u.clickOverview(x, y, reg)
	case reg.Selected.Contains(x, y):
		u.clickSelected(x, y, reg)
	case reg.Targets.Contains(x, y):
		u.clickTargets(x, y, reg)
	case reg.Modules.Contains(x, y):
		u.clickModules(x, y, reg)
	}
}

// clickOverview handles header sorting and row selection.
func (u *UI) clickOverview(x, y float64, reg Regions) {
	r := reg.Overview
	if y < r.Y+2+ovHeaderH {
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
	row := OverviewRowAt(r, y)
	if row < 0 {
		return
	}
	rows := u.overviewRows()
	if row < len(rows) {
		u.Selected = rows[row].Entity
	}
}

// clickSelected runs the contextual action button under the cursor.
func (u *UI) clickSelected(x, y float64, reg Regions) {
	info, ok := u.s.Info(u.Selected)
	if !ok {
		return
	}
	acts := ActionsFor(info)
	for i, ar := range ActionRects(reg.Selected, len(acts)) {
		if ar.Contains(x, y) {
			u.doAction(acts[i])
			return
		}
	}
}

// clickTargets selects the target box under the cursor.
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
func (u *UI) clickModules(x, y float64, reg Regions) {
	row, i := ModuleCellAt(reg.Modules, x, y)
	if row < 0 {
		return
	}
	switch row {
	case 0:
		u.toggleModule(content.SlotHigh, i)
	case 1:
		u.toggleModule(content.SlotMid, i)
	default:
		u.toggleModule(content.SlotLow, i)
	}
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// overviewRows returns the sorted grid rows backing the overview table.
func (u *UI) overviewRows() []sim.Info {
	rows := append([]sim.Info(nil), u.s.Grid()...)
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
			u.enqueue(sim.CmdWarpTo{Target: u.Selected})
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

// nearest returns the closest grid entity satisfying match.
func (u *UI) nearest(match func(sim.Info) bool) (sim.Info, bool) {
	var best sim.Info
	found := false
	for _, inf := range u.s.Grid() {
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
// cycling over every lockable ship on grid.
func (u *UI) cycleLock() {
	rows := append([]sim.Info(nil), u.s.Grid()...)
	var ships []sim.Info
	player := u.s.Player()
	for _, inf := range rows {
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
		u.drawStation(dst, s, reg)
	} else {
		u.drawOverview(dst, s, reg)
		u.drawSelected(dst, s, reg)
		u.drawHUD(dst, s, reg)
		u.drawModules(dst, s, reg)
	}
	u.drawLog(dst, reg)
	if u.showInv {
		u.drawInventory(dst, s, reg)
	}
	if u.showFit {
		u.drawFitting(dst, s, reg)
	}
}
