package uikit

import (
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
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
			if got, want := r.cells[row][i].btn.Text().Label, r.shortName(def); got != want {
				t.Errorf("cell(%d,%d) label = %q, want %q", row, i, got, want)
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
	// Empty slots carry no label and no tooltip.
	empty := r.cells[2][3]
	if lbl := empty.btn.Text().Label; lbl != "" {
		t.Errorf("empty slot label = %q, want empty", lbl)
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

// TestRackTooltipStats pins the hover tooltip to the module data: name,
// slot, weapon/mining numbers and the currently preferred charge.
func TestRackTooltipStats(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(r.Container(), r)
	c.frame(t, 1280, 720)

	gun := r.cells[0][0].tipText.Label
	for _, want := range []string{
		"EMP Turret",
		"High slot 1",
		"Damage 24.0 per shot",
		"Mix  EM 16.0  TH 5.0  KN 2.0  EX 1.0",
		"Optimal 12.0 km  Falloff 6.0 km",
		"Tracking 145.00 rad/s",
		"Charge: Auto",
		"Uses 2.0 GJ per cycle",
	} {
		if !strings.Contains(gun, want) {
			t.Errorf("gun tooltip missing %q:\n%s", want, gun)
		}
	}
	mining := r.cells[0][1].tipText.Label
	if !strings.Contains(mining, "Mining Laser") || !strings.Contains(mining, "Yields ") ||
		!strings.Contains(mining, "m³ / ") {
		t.Errorf("mining tooltip wrong:\n%s", mining)
	}
	// Boost numbers on the shield booster (mid slot 2).
	booster := r.cells[1][1].tipText.Label
	if !strings.Contains(booster, "Shield Booster") || !strings.Contains(booster, "Shield +") {
		t.Errorf("booster tooltip wrong:\n%s", booster)
	}
	// Empty slots keep their tooltip empty (the widget drops the ToolTips).
	if got := r.cells[2][2].tipText.Label; got != "" {
		t.Errorf("empty slot tooltip = %q, want empty", got)
	}
}

// TestRackRightClickCyclesAmmo is the command-wiring test: right-clicking
// the EMP turret walks Auto → the four light charges in cargo (item-id
// order) → Auto, each step enqueuing one CmdSetAmmo; non-weapon cells
// never enqueue.
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
	wantItems := []content.ItemID{"charge-ap", "charge-emp", "charge-fusion", "charge-plutonium", ""}
	for i, want := range wantItems {
		before := len(*cmds)
		rightClick(c, t, gun.btn)
		if len(*cmds) != before+1 {
			t.Fatalf("click %d enqueued %d commands, want 1", i, len(*cmds)-before)
		}
		cmd, ok := (*cmds)[before].(sim.CmdSetAmmo)
		if !ok {
			t.Fatalf("click %d command = %T, want sim.CmdSetAmmo", i, (*cmds)[before])
		}
		if cmd.Slot != slot || cmd.Item != want {
			t.Fatalf("click %d = CmdSetAmmo%+v, want {Slot:%+v Item:%q}", i, cmd, slot, want)
		}
		if r.ammoPref[slot] != want {
			t.Fatalf("local ammo mirror = %q, want %q", r.ammoPref[slot], want)
		}
		// The tooltip follows the preference on the next Sync.
		r.Sync()
		if line := gun.tipText.Label; !strings.Contains(line, "Charge: "+r.ammoName(want)) {
			t.Fatalf("tooltip after click %d lacks the charge line:\n%s", i, line)
		}
	}

	// The mining laser and the ammo-less rocket pod never cycle.
	for _, idx := range []int{1, 2} {
		before := len(*cmds)
		rightClick(c, t, r.cells[0][idx].btn)
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
