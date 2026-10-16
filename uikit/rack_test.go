package uikit

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/hajimehoshi/ebiten/v2"
)

// rightClick fires the widget-level right-button click ebitenui would fire
// from input, then flushes the deferred event queue through an Update.
func rightClick(c *chromeUI, t *testing.T, btn *widget.Button) {
	t.Helper()
	w := btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: w,
		Button: ebiten.MouseButtonRight,
	})
	c.update(t)
}

// TestRackConstruction builds the rack, checks every cell keeps the 40 px
// minimum hit area, and verifies the showcase sloop's 3/3/2 fit renders
// exactly the fitted cells (the trailing low slots stay hidden).
func TestRackConstruction(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	if r.Container() == nil {
		t.Fatal("Rack.Container() is nil")
	}
	for row := range r.cells {
		for _, cell := range r.cells[row] {
			w := cell.btn.GetWidget()
			if w.MinWidth < 40 || w.MinHeight < 40 {
				t.Errorf("slot %v hit area %dx%d, want at least 40x40",
					cell.slot, w.MinWidth, w.MinHeight)
			}
		}
	}

	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720) // Sync populates, Update validates the tree

	// sloop: 3 high, 3 mid, 2 low → low cells 3 and 4 are empty slots.
	for _, idx := range []int{0, 1, 2} {
		if vis := r.cells[0][idx].root.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
			t.Errorf("high cell %d hidden (visibility %d)", idx, vis)
		}
	}
	for _, idx := range []int{0, 1} {
		if vis := r.cells[2][idx].root.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
			t.Errorf("low cell %d hidden (visibility %d)", idx, vis)
		}
	}
	for _, idx := range []int{2, 3} {
		if vis := r.cells[2][idx].root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
			t.Errorf("empty low cell %d shown (visibility %d)", idx, vis)
		}
	}

	// Module short names: first word of the module name per cell.
	for row, ids := range map[int][]content.ModuleID{
		0: {"mod-emp-turret", "mod-mining-laser", "mod-rocket-pod"},
		1: {"mod-afterburner", "mod-shield-boost", "mod-stasis-web"},
		2: {"mod-armor-rep", "mod-cap-recharger"},
	} {
		for i, id := range ids {
			def, ok := env.Content.Module(id)
			if !ok {
				t.Fatalf("module %s missing from content", id)
			}
			cell := r.cells[row][i]
			// The name lives on the line under the tile now.
			if got, want := cell.name.Label, r.shortName(def); got != want {
				t.Errorf("cell(%d,%d) name = %q, want %q", row, i, got, want)
			}
			// The tile carries the module's class glyph (the same cached
			// image the icon registry hands everyone else).
			wantGlyph := icons.Get(ModuleIcon(def.Kind), IconButton, env.Pal().Text)
			if cell.btn.GraphicImage() == nil || cell.btn.GraphicImage().Idle != wantGlyph {
				t.Errorf("cell(%d,%d) tile glyph is not %s", row, i, ModuleIcon(def.Kind))
			}
		}
	}
	// The short name is the first word; long ones ellipsize into the cell.
	if got := r.shortName(&content.ModuleDef{Name: "EMP Turret"}); got != "EMP" {
		t.Errorf("shortName(EMP Turret) = %q, want EMP", got)
	}
	if got := r.shortName(nil); got != "" {
		t.Errorf("shortName(nil) = %q, want empty", got)
	}
	long := r.shortName(&content.ModuleDef{Name: "Overengineered Reactor Grid"})
	if !strings.HasSuffix(long, "…") {
		t.Errorf("long shortName = %q, want an ellipsized form", long)
	}
	if w := r.measure(long); w > rackCellW-6 {
		t.Errorf("long shortName measures %v > cell width %d", w, rackCellW-6)
	}
	// Empty slots carry no name, no glyph and no tooltip.
	empty := r.cells[2][3]
	if lbl := empty.name.Label; lbl != "" {
		t.Errorf("empty slot name = %q, want empty", lbl)
	}
	if g := empty.btn.GraphicImage(); g == nil || g.Idle != icons.Get(icons.Fallback, IconButton, color.RGBA{}) {
		t.Error("empty slot tile should hold a transparent box, not a glyph")
	}
	if tips := empty.btn.GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("empty slot has %d tooltips, want 0", len(tips))
	}
}

// TestRackActiveAndCycle covers the runtime states: inactive modules wear
// the disabled look, active ones light up with a cycle progress bar read
// from ModuleRuntime.Active/CycleEnds against Sim.Time().
func TestRackActiveAndCycle(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	cell := r.cells[0][0] // EMP Turret, activation time 3 s
	if !cell.btn.GetWidget().Disabled {
		t.Fatal("inactive module is not disabled")
	}
	if cell.bar.GetCurrent() != 0 {
		t.Errorf("idle cycle bar = %d, want 0", cell.bar.GetCurrent())
	}

	fit, _ := env.Sim.Fittings.Get(env.Sim.Player())
	rt := &fit.HighRT[0]
	rt.Active = true
	rt.CycleEnds = env.Sim.Time() + 1.5 // half of the 3 s cycle elapsed
	r.Sync()
	if cell.btn.GetWidget().Disabled {
		t.Error("active module is disabled")
	}
	// (now - (CycleEnds - ActivationTime)) / ActivationTime = 1.5 / 3
	if got := cell.bar.GetCurrent(); got != rackCycleMax/2 {
		t.Errorf("cycle bar = %d, want %d", got, rackCycleMax/2)
	}

	rt.Active = false
	r.Sync()
	if !cell.btn.GetWidget().Disabled {
		t.Error("deactivated module is not disabled")
	}
	if got := cell.bar.GetCurrent(); got != 0 {
		t.Errorf("deactivated cycle bar = %d, want 0", got)
	}
}

// TestRackTooltipStats pins the hover tooltip to the module data: the
// structured model carries the name, the slot line, every number the old
// plain-text tooltip carried and the currently preferred charge.
func TestRackTooltipStats(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	gun := r.cells[0][0].tip.view.Model()
	if gun.Icon != "mod_weapon" || gun.Title != "EMP Turret" || gun.Sub != "High slot 1 · Weapon" {
		t.Errorf("gun header = %q/%q/%q, want mod_weapon / EMP Turret / High slot 1 · Weapon",
			gun.Icon, gun.Title, gun.Sub)
	}
	// Every line the plain-text tooltip had, as rows.
	for _, want := range []struct{ sec, label, value string }{
		{"DAMAGE", "Per shot", "24.0"},
		{"DAMAGE", "Electro-magnetic", "16.0"},
		{"DAMAGE", "Thermal", "5.0"},
		{"DAMAGE", "Kinetic", "2.0"},
		{"DAMAGE", "Explosive", "1.0"},
		{"RANGE", "Optimal", "12.0 km"},
		{"RANGE", "Falloff", "6.0 km"},
		{"FIRING", "Tracking", "145.00 rad/s"},
		{"CHARGE", "Loaded", "Auto"},
		{"ACTIVATION", "Cycle", "3.0 s"},
		{"ACTIVATION", "Cap use", "2.0 GJ"},
	} {
		if got := tipValue(gun, want.sec, want.label); got != want.value {
			t.Errorf("gun %s/%s = %q, want %q (sections %v)",
				want.sec, want.label, got, want.value, tipSectionLabels(gun))
		}
	}
	if len(gun.Hints) != 2 || gun.Hints[0] != "Right-click: module menu" || gun.Hints[1] != "Click: activate" {
		t.Errorf("gun hints = %q, want the menu and activate footers", gun.Hints)
	}

	// Mining laser: yield, cycle and capacitor draw — no charge section.
	mining := r.cells[0][1].tip.view.Model()
	if got := tipValue(mining, "YIELD", "Per cycle"); got != "100.0 m³" {
		t.Errorf("miner yield = %q, want 100.0 m³", got)
	}
	if got := tipValue(mining, "ACTIVATION", "Cycle"); got != "30.0 s" {
		t.Errorf("miner cycle = %q, want 30.0 s", got)
	}
	if tipSection(mining, "CHARGE") != nil {
		t.Error("mining laser carries a CHARGE section")
	}
	if mining.Sub != "High slot 2 · Mining laser" {
		t.Errorf("miner sub = %q", mining.Sub)
	}

	// Shield booster: the repair block, tinted as a gain, plus its cycle.
	booster := r.cells[1][1].tip.view.Model()
	rep := tipSection(booster, "REPAIR")
	if rep == nil || len(rep.Rows) != 1 || rep.Rows[0].Value != "+60.0 HP" {
		t.Fatalf("booster repair section = %+v, want Shield +60.0 HP", rep)
	}
	if rep.Rows[0].Tint != DefaultPalette().Good {
		t.Errorf("repair value tint = %v, want ColGood", rep.Rows[0].Tint)
	}
	if got := tipValue(booster, "ACTIVATION", "Cycle"); got != "4.0 s" {
		t.Errorf("booster cycle = %q, want 4.0 s", got)
	}

	// Stasis web: the signed target modifier is tinted by its sign.
	web := r.cells[1][2].tip.view.Model()
	eff := tipSection(web, "EFFECT")
	if eff == nil || len(eff.Rows) != 1 || eff.Rows[0].Value != "−45%" {
		t.Fatalf("web effect section = %+v, want Target speed −45%%", eff)
	}
	if eff.Rows[0].Tint != DefaultPalette().Danger {
		t.Errorf("web value tint = %v, want ColDanger", eff.Rows[0].Tint)
	}

	// The menu footer promises what a right-click opens (the module menu
	// carries Cycle charge, gated to where it can act): stripping the
	// cargo of that weapon's charges disarms Cycle charge but keeps the
	// menu — Show Info still works (the charge block stays, reads Auto).
	cg, ok := env.Sim.Cargos.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no cargo")
	}
	cg.Stacks = nil
	r.Sync()
	gun = r.cells[0][0].tip.view.Model()
	if len(gun.Hints) != 2 || gun.Hints[0] != "Right-click: module menu" || gun.Hints[1] != "Click: activate" {
		t.Errorf("disarmed gun hints = %q, want the menu and activate footers", gun.Hints)
	}
	if tipValue(gun, "CHARGE", "Loaded") != "Auto" {
		t.Errorf("charge block = %+v, want Loaded Auto", tipSection(gun, "CHARGE"))
	}

	// Empty slots keep their tooltip empty (the widget drops the ToolTips).
	empty := r.cells[2][2]
	if tips := empty.btn.GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("empty slot has %d tooltips, want 0", len(tips))
	}
	if got := empty.tip.view; got != nil {
		t.Error("empty slot built a tooltip panel")
	}
}

// TestRackRightClickCyclesAmmo is the command-wiring test: right-clicking
// the EMP turret opens its context menu (Show Info first and bold, then
// the charge ref, a separator and Cycle charge), and picking Cycle charge
// walks Auto → the four light charges in cargo (item-id order) → Auto,
// each step enqueuing exactly one CmdSetAmmo — the same command the old
// direct right-click issued. Non-weapon cells never enqueue (their menu
// has no cycle entry at all).
func TestRackRightClickCyclesAmmo(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720) // Sync + Update registers the click handlers

	// Candidates: Auto first, then the light charges in cargo by item id.
	wantCands := []content.ItemID{"", "charge-ap", "charge-emp", "charge-fusion", "charge-plutonium"}
	got := r.ammoCandidates("light")
	if len(got) != len(wantCands) {
		t.Fatalf("candidates = %v, want %v", got, wantCands)
	}
	for i := range wantCands {
		if got[i] != wantCands[i] {
			t.Fatalf("candidates = %v, want %v", got, wantCands)
		}
	}

	gun := r.cells[0][0] // EMP Turret, high slot 1
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}

	// The first menu: Show Info leads (bold), then the separator and
	// Cycle charge (no charge preferred yet → no charge-ref row).
	rightClick(c, t, gun.btn)
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("rack right-click opened no menu")
	}
	if ents := m.Entries(); len(ents) != 3 ||
		ents[0].Label != "Show Info" || !ents[0].Bold || ents[0].Disabled ||
		!ents[1].Sep ||
		ents[2].Label != "Cycle charge" || ents[2].Disabled {
		t.Fatalf("rack menu entries = %+v", ents)
	}
	m.Esc()

	wantItems := []content.ItemID{"charge-ap", "charge-emp", "charge-fusion", "charge-plutonium", ""}
	for i, want := range wantItems {
		before := len(*cmds)
		pickMenuEntry(t, c, env, gun.btn, "Cycle charge")
		if len(*cmds) != before+1 {
			t.Fatalf("step %d enqueued %d commands, want 1", i, len(*cmds)-before)
		}
		cmd, ok := (*cmds)[before].(sim.CmdSetAmmo)
		if !ok {
			t.Fatalf("step %d command = %T, want sim.CmdSetAmmo", i, (*cmds)[before])
		}
		if cmd.Slot != slot || cmd.Item != want {
			t.Fatalf("step %d = CmdSetAmmo%+v, want {Slot:%+v Item:%q}", i, cmd, slot, want)
		}
		if r.ammoPref[slot] != want {
			t.Fatalf("local ammo mirror = %q, want %q", r.ammoPref[slot], want)
		}
		// The tooltip follows the preference on the next Sync.
		r.Sync()
		if got := tipValue(gun.tip.view.Model(), "CHARGE", "Loaded"); got != r.ammoName(want) {
			t.Fatalf("tooltip after step %d shows charge %q, want %q", i, got, r.ammoName(want))
		}
	}
	// With a charge preferred, the menu carries its Show Info ref too
	// (Sync rebuilds the cells' declared specs — production syncs every
	// frame, so the menu always reads the current preference).
	r.ammoPref[slot] = "charge-fusion"
	r.Sync()
	rightClick(c, t, gun.btn)
	if ents := env.Menu().Entries(); len(ents) != 4 || ents[1].Label != "Show Info — "+r.ammoName("charge-fusion") {
		t.Fatalf("menu with a preferred charge = %+v", ents)
	}
	env.Menu().Esc()
	r.ammoPref[slot] = ""

	// The mining laser and the ammo-less rocket pod never enqueue: their
	// menus carry no Cycle charge entry to press.
	for _, idx := range []int{1, 2} {
		before := len(*cmds)
		rightClick(c, t, r.cells[0][idx].btn)
		for _, e := range env.Menu().Entries() {
			if e.Label == "Cycle charge" {
				t.Fatalf("non-weapon cell %d menu offers Cycle charge", idx)
			}
		}
		env.Menu().Click(env.Menu().RowRect(0).Min) // any press closes it
		if len(*cmds) != before {
			t.Fatalf("non-weapon cell %d enqueued %d commands", idx, len(*cmds)-before)
		}
	}
}

// TestRackDrawSmoke draws the rack at both design sizes inside an
// interface root; the panel must anchor bottom-right.
func TestRackDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	rect := r.Container().GetWidget().Rect
	if rect.Max.X < 1269 || rect.Max.X > 1271 || rect.Max.Y < 709 || rect.Max.Y > 711 {
		t.Errorf("rack rect %v, want a bottom-right corner at (1270,710)", rect)
	}
	c.smoke(t)
}

// leftClick fires the widget-level left-button click ebitenui would fire
// from input, then flushes the deferred event queue through an Update.
func leftClick(c *chromeUI, t *testing.T, btn *widget.Button) {
	t.Helper()
	w := btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: w,
		Button: ebiten.MouseButtonLeft,
	})
	c.update(t)
}

// spendDouble clears the rack's double-click arming so the next click in
// a test reads as a lone single click: the clicks here run microseconds
// apart, well inside the gesture window, where production clicks are
// human-paced.
func spendDouble(r *Rack) { r.dbl.armed = false }

// TestRackLeftClickToggles is the command-wiring test for defect 3:
// left-click enqueues CmdDeactivate for a running module and CmdActivate
// for an idle one (target resolved from the locks), empty slots are inert,
// and the active/inactive visual follows ModuleRuntime through Sync.
func TestRackLeftClickToggles(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	s := env.Sim
	gun := r.cells[0][0] // EMP Turret, high slot 1 (a weapon)
	miner := r.cells[0][1]
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}

	// Empty slots are inert: no fitting, no enqueue.
	if len(*cmds) != 0 {
		t.Fatalf("precondition: %d commands already queued", len(*cmds))
	}
	leftClick(c, t, r.cells[2][3].btn)
	if len(*cmds) != 0 {
		t.Fatalf("empty slot enqueued %d commands, want 0", len(*cmds))
	}

	// Inactive weapon, no lock: CmdActivate with the zero (self) target.
	leftClick(c, t, gun.btn)
	if len(*cmds) != 1 {
		t.Fatalf("idle click enqueued %d commands, want 1", len(*cmds))
	}
	act, ok := (*cmds)[0].(sim.CmdActivate)
	if !ok {
		t.Fatalf("command = %T, want sim.CmdActivate", (*cmds)[0])
	}
	if act.Slot != slot || act.Target != 0 {
		t.Fatalf("CmdActivate = %+v, want {Slot:%+v Target:0}", act, slot)
	}

	// With locks, the activation targets the first lock that suits the
	// module: a ship for the weapon, an asteroid for the miner.
	var shipEnt, rockEnt = ecs.Entity(0), ecs.Entity(0)
	for _, inf := range s.Grid() {
		if inf.Kind == sim.KindShip && shipEnt == 0 {
			shipEnt = inf.Entity
		} else if inf.Kind == sim.KindAsteroid && rockEnt == 0 {
			rockEnt = inf.Entity
		}
	}
	tg, ok := s.Targetings.Get(s.Player())
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{
		{Target: rockEnt, Progress: 1}, // rock locked first: weapon skips it
		{Target: shipEnt, Progress: 1},
	}
	r.Sync()

	before := len(*cmds)
	spendDouble(r) // next gun click is a lone single, not a re-click
	leftClick(c, t, gun.btn)
	if len(*cmds) != before+1 {
		t.Fatalf("locked weapon click enqueued %d commands, want 1", len(*cmds)-before)
	}
	if act, ok := (*cmds)[before].(sim.CmdActivate); !ok || act.Target != shipEnt {
		t.Fatalf("weapon target = %+v (%T), want the locked ship", (*cmds)[before], (*cmds)[before])
	}

	before = len(*cmds)
	leftClick(c, t, miner.btn)
	if act, ok := (*cmds)[before].(sim.CmdActivate); !ok ||
		act.Slot != (sim.SlotRef{Class: content.SlotHigh, Index: 1}) || act.Target != rockEnt {
		t.Fatalf("miner command = %+v (%T), want CmdActivate on the locked rock", (*cmds)[before], (*cmds)[before])
	}

	// Active module: click enqueues CmdDeactivate, and the visual state
	// (button Disabled = dim cell) follows the runtime through Sync.
	fit, ok := s.Fittings.Get(s.Player())
	if !ok {
		t.Fatal("player has no fitting")
	}
	rt := &fit.HighRT[0]
	rt.Active = true
	r.Sync()
	if gun.btn.GetWidget().Disabled {
		t.Fatal("active module wears the inactive (dim) look")
	}
	before = len(*cmds)
	leftClick(c, t, gun.btn)
	if len(*cmds) != before+1 {
		t.Fatalf("active click enqueued %d commands, want 1", len(*cmds)-before)
	}
	if de, ok := (*cmds)[before].(sim.CmdDeactivate); !ok || de.Slot != slot {
		t.Fatalf("command = %+v (%T), want CmdDeactivate{Slot:%+v}", (*cmds)[before], (*cmds)[before], slot)
	}
	rt.Active = false
	r.Sync()
	if !gun.btn.GetWidget().Disabled {
		t.Error("deactivated module lost the inactive (dim) look")
	}
}

// TestRackClickTargetsSelection pins the F-key parity seam: click
// activation targets the player's SELECTED entity (via SetTargetPicker)
// when it is locked and suits the module, falling back to the first
// suiting lock when the selection is zero.
func TestRackClickTargetsSelection(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	s := env.Sim
	var ships []ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Kind == sim.KindShip {
			ships = append(ships, inf.Entity)
		}
	}
	if len(ships) < 2 {
		t.Fatalf("fixture has %d ships, need >= 2", len(ships))
	}
	first, second := ships[0], ships[1]
	tg, ok := s.Targetings.Get(s.Player())
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{
		{Target: first, Progress: 1},
		{Target: second, Progress: 1},
	}
	selected := second
	r.SetTargetPicker(func() ecs.Entity { return selected })
	r.Sync()

	gun := r.cells[0][0] // EMP Turret: a weapon
	before := len(*cmds)
	leftClick(c, t, gun.btn)
	if act, ok := (*cmds)[before].(sim.CmdActivate); !ok || act.Target != second {
		t.Fatalf("selected-target click = %+v (%T), want CmdActivate on the selected ship %d",
			(*cmds)[before], (*cmds)[before], second)
	}

	// Selection cleared: fall back to the first suiting lock.
	selected = 0
	spendDouble(r) // next gun click is a lone single, not a re-click
	before = len(*cmds)
	leftClick(c, t, gun.btn)
	if act, ok := (*cmds)[before].(sim.CmdActivate); !ok || act.Target != first {
		t.Fatalf("fallback click = %+v (%T), want CmdActivate on the first lock %d",
			(*cmds)[before], (*cmds)[before], first)
	}
}

// TestRackCellStacksTileNameAndBar pins the cell column: the icon tile on
// top (its 40 px hit area intact), the module's name under it and the
// cycle bar at the bottom — three disjoint rects, all inside the cell.
func TestRackCellStacksTileNameAndBar(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	for row := range r.cells {
		for idx, cell := range r.cells[row] {
			if cell.root.GetWidget().GetVisibility() != widget.Visibility_Show {
				continue // hidden slots have no rects to check
			}
			tile := cell.btn.GetWidget().Rect
			name := cell.name.GetWidget().Rect
			bar := cell.bar.GetWidget().Rect
			root := cell.root.GetWidget().Rect
			if tile.Dy() < rackCellH || tile.Dx() < rackCellW {
				t.Errorf("cell(%d,%d) tile = %v, want at least %d×%d",
					row, idx, tile, rackCellW, rackCellH)
			}
			if name.Dy() <= 0 {
				t.Errorf("cell(%d,%d) name line = %v, want a measured line", row, idx, name)
			}
			for _, pair := range [][2]image.Rectangle{{tile, name}, {name, bar}, {tile, bar}} {
				if pair[0].Overlaps(pair[1]) {
					t.Errorf("cell(%d,%d): %v overlaps %v", row, idx, pair[0], pair[1])
				}
			}
			if tile.Max.Y != name.Min.Y || name.Max.Y != bar.Min.Y {
				t.Errorf("cell(%d,%d) stack = tile %v / name %v / bar %v, want contiguous",
					row, idx, tile, name, bar)
			}
			if !tile.In(root) || !name.In(root) || !bar.In(root) {
				t.Errorf("cell(%d,%d) children escape the cell %v", row, idx, root)
			}
		}
	}
}

// TestRackCellSpecData: what each cell DECLARES (its RowSpec) — the
// fitted weapon declares its module Ref, the Cycle charge action and the
// tooltip payload (and gains the charge's secondary ref once one is
// preferred), a non-weapon declares no Cycle charge, and an empty slot
// declares nothing at all (no menu, no tooltip).
func TestRackCellSpecData(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	gun := r.cells[0][0] // EMP Turret: a weapon with the "light" ammo class
	if gun.spec.Ref == nil || *gun.spec.Ref != ModuleRef("mod-emp-turret") {
		t.Fatalf("gun Ref = %v, want module/mod-emp-turret", gun.spec.Ref)
	}
	if gun.spec.SecRef != nil {
		t.Errorf("gun declares a secondary ref before any charge is preferred: %v", *gun.spec.SecRef)
	}
	if len(gun.spec.Actions) != 1 || gun.spec.Actions[0].Label != "Cycle charge" {
		t.Fatalf("gun actions = %+v, want the single Cycle charge", gun.spec.Actions)
	}
	if gun.spec.Actions[0].Disabled {
		t.Error("Cycle charge grayed with light charges in cargo")
	}
	if gun.spec.Tip.Key == "" || gun.spec.Tip.Build == nil {
		t.Error("gun declares no tooltip payload")
	}

	// A charge preference appears as the secondary ref after a Sync
	// (production syncs every frame, so the menu always reads it).
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}
	r.ammoPref[slot] = "charge-fusion"
	r.Sync()
	if gun.spec.SecRef == nil || *gun.spec.SecRef != ItemRef("charge-fusion") {
		t.Errorf("gun SecRef = %v, want item/charge-fusion", gun.spec.SecRef)
	}
	r.ammoPref[slot] = ""
	r.Sync()

	// Non-weapons declare no Cycle charge entry at all.
	miner := r.cells[0][1]
	if len(miner.spec.Actions) != 0 {
		t.Errorf("mining laser actions = %+v, want none", miner.spec.Actions)
	}
	if miner.spec.Ref == nil {
		t.Error("mining laser declares no Ref")
	}

	// Empty slot: the zero spec — nothing to menu, nothing to hover.
	empty := r.cells[2][3]
	if empty.spec.Ref != nil || empty.spec.SecRef != nil ||
		len(empty.spec.Actions) != 0 || empty.spec.Tip.Build != nil {
		t.Errorf("empty slot spec = %+v, want the zero RowSpec", empty.spec)
	}
}

// TestRackDoubleClickShowsInfoWithoutToggle pins the rack's strict
// double-click: a lone click toggles (single-click activation preserved),
// the repeat click inside the window opens Show Info and does NOT toggle,
// and the spent gesture means the next click toggles again.
func TestRackDoubleClickShowsInfoWithoutToggle(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	gun := r.cells[0][0]
	if len(*cmds) != 0 {
		t.Fatalf("fixture: %d commands queued", len(*cmds))
	}

	// A lone click toggles (single-click activation, preserved exactly).
	leftClick(c, t, gun.btn)
	if len(*cmds) != 1 {
		t.Fatalf("single click queued %d commands, want 1 (the toggle)", len(*cmds))
	}
	if _, ok := (*cmds)[0].(sim.CmdActivate); !ok {
		t.Fatalf("command = %T, want sim.CmdActivate", (*cmds)[0])
	}

	// The repeat click inside the window: Show Info, strictly — the
	// dual gesture is gone, this click does not toggle.
	leftClick(c, t, gun.btn)
	if !env.InfoWindow().IsOpen() {
		t.Fatal("double-click did not open Show Info")
	}
	if got, _ := env.InfoWindow().Ref(); got != ModuleRef("mod-emp-turret") {
		t.Errorf("Show Info ref = %v, want module/mod-emp-turret", got)
	}
	if len(*cmds) != 1 {
		t.Fatalf("double-click queued %d commands, want 1 — the repeat click must not toggle", len(*cmds))
	}

	// The gesture is spent: the next click is a lone click again.
	leftClick(c, t, gun.btn)
	if len(*cmds) != 2 {
		t.Fatalf("click after the spent gesture queued %d commands, want 2 (toggling again)", len(*cmds))
	}
}
