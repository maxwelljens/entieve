package screens

import (
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"

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
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720) // Sync populates, Update validates the tree

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
		0: {"mod-autocannon-s", "mod-mining-laser", "mod-rocket-pod"},
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
			wantGlyph := icons.Get(theme.ModuleIcon(def.Kind), theme.IconButton, env.Pal().Text)
			if cell.btn.GraphicImage() == nil || cell.btn.GraphicImage().Idle != wantGlyph {
				t.Errorf("cell(%d,%d) tile glyph is not %s", row, i, theme.ModuleIcon(def.Kind))
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
	if g := empty.btn.GraphicImage(); g == nil || g.Idle != icons.Get(icons.Fallback, theme.IconButton, color.RGBA{}) {
		t.Error("empty slot tile should hold a transparent box, not a glyph")
	}
	if tips := empty.btn.GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("empty slot has %d tooltips, want 0", len(tips))
	}
}

// TestRackActiveAndCycle covers the runtime states: idle modules wear the
// disabled look and no ring, active ones light up with a cycle ring read
// from ModuleRuntime.Active/CycleEnds against Sim.Time(), and a CANCELLED
// module keeps its clock — its ring stays visible but greyed until the
// boundary passes, so cancelled never reads as plain idle.
func TestRackActiveAndCycle(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	cell := r.cells[0][0] // EMP Turret, activation time 3 s
	if !cell.btn.GetWidget().Disabled {
		t.Fatal("inactive module is not disabled")
	}
	if cell.ringOn || cell.ringFrac != 0 {
		t.Errorf("inactive module ring = on:%v frac:%v, want no ring", cell.ringOn, cell.ringFrac)
	}
	if vis := cell.ringGfx.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("inactive ring visibility = %d, want hidden", vis)
	}

	fit, _ := env.Sim.Fittings.Get(env.Sim.Player())
	rt := &fit.HighRT[0]
	rt.Active = true
	rt.CycleEnds = env.Sim.Time() + 1.5 // half of the 3 s cycle remaining
	r.Sync()
	if cell.btn.GetWidget().Disabled {
		t.Error("active module is disabled")
	}
	// remaining = 1 − (now − (CycleEnds − ActivationTime)) / ActivationTime
	// = 1.5 / 3
	if !cell.ringOn || cell.ringFrac != 0.5 || !cell.ringLit {
		t.Errorf("cycle ring = on:%v frac:%v lit:%v, want lit on / 0.5", cell.ringOn, cell.ringFrac, cell.ringLit)
	}
	if vis := cell.ringGfx.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Errorf("active ring visibility = %d, want shown", vis)
	}

	// Clockwise depletion: as now−start grows the remaining fraction shrinks.
	rt.CycleEnds = env.Sim.Time() + 0.75
	r.Sync()
	if cell.ringFrac != 0.25 {
		t.Errorf("cycle ring after three quarters = %v, want 0.25", cell.ringFrac)
	}
	rt.CycleEnds = env.Sim.Time() + 0.001
	r.Sync()
	if cell.ringFrac != 0 {
		t.Errorf("cycle ring at the end of the window = %v, want 0", cell.ringFrac)
	}

	rt.Active = false
	r.Sync()
	if !cell.btn.GetWidget().Disabled {
		t.Error("deactivated module is not disabled")
	}
	// The cancelled cycle still runs out (CycleEnds is ahead of now): the
	// ring stays visible but GREYED — cancelled-with-cooldown reads
	// differently from plain idle.
	if !cell.ringOn || cell.ringFrac != 0 || cell.ringLit {
		t.Errorf("cancelled ring = on:%v frac:%v lit:%v, want greyed ring at 0", cell.ringOn, cell.ringFrac, cell.ringLit)
	}
	if vis := cell.ringGfx.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Errorf("cancelled ring visibility = %d, want shown", vis)
	}

	// Once the cancelled boundary passes, the module is plain idle: no ring.
	rt.CycleEnds = env.Sim.Time()
	r.Sync()
	if cell.ringOn {
		t.Error("ring after the cancelled cycle ran out, want none")
	}
	if vis := cell.ringGfx.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("ring visibility after the boundary = %d, want hidden", vis)
	}
}

// TestRackTooltipStats pins the hover tooltip to the module data: the
// structured model carries the name, the slot line, every number the old
// plain-text tooltip carried and the currently preferred charge.
func TestRackTooltipStats(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0].tip.View.Model()
	if gun.Icon != "mod_weapon" || gun.Title != "Small Autocannon" || gun.Sub != "High slot 1 · Weapon" {
		t.Errorf("gun header = %q/%q/%q, want mod_weapon / Small Autocannon / High slot 1 · Weapon",
			gun.Icon, gun.Title, gun.Sub)
	}
	// Every line the plain-text tooltip had, as rows.
	for _, want := range []struct{ sec, label, value string }{
		{"DAMAGE", "Per shot", "25.0"},
		{"DAMAGE", "Electro-magnetic", "8.0"},
		{"DAMAGE", "Thermal", "6.0"},
		{"DAMAGE", "Kinetic", "6.0"},
		{"DAMAGE", "Explosive", "5.0"},
		{"RANGE", "Optimal", "4.0 km"},
		{"RANGE", "Falloff", "6.0 km"},
		{"FIRING", "Tracking", "160.00 rad/s"},
		{"CHARGE", "Loaded", "None"},
		{"ACTIVATION", "Cycle", "3.0 s"},
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
	mining := r.cells[0][1].tip.View.Model()
	if got := tipValue(mining, "YIELD", "Per cycle"); got != "100 m³" {
		t.Errorf("miner yield = %q, want 100 m³", got)
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
	booster := r.cells[1][1].tip.View.Model()
	rep := tipSection(booster, "REPAIR")
	if rep == nil || len(rep.Rows) != 1 || rep.Rows[0].Value != "+60.0 HP" {
		t.Fatalf("booster repair section = %+v, want Shield +60.0 HP", rep)
	}
	if rep.Rows[0].Tint != theme.DefaultPalette().Good {
		t.Errorf("repair value tint = %v, want ColGood", rep.Rows[0].Tint)
	}
	if got := tipValue(booster, "ACTIVATION", "Cycle"); got != "4.0 s" {
		t.Errorf("booster cycle = %q, want 4.0 s", got)
	}

	// Stasis web: the signed target modifier is tinted by its sign.
	web := r.cells[1][2].tip.View.Model()
	eff := tipSection(web, "EFFECT")
	if eff == nil || len(eff.Rows) != 1 || eff.Rows[0].Value != "−45%" {
		t.Fatalf("web effect section = %+v, want Target speed −45%%", eff)
	}
	if eff.Rows[0].Tint != theme.DefaultPalette().Danger {
		t.Errorf("web value tint = %v, want ColDanger", eff.Rows[0].Tint)
	}

	// The menu footer promises what a right-click opens (the module menu
	// carries Cycle charge, gated to where it can act): stripping the
	// cargo of that weapon's charges disarms Cycle charge but keeps the
	// menu — Show Info still works (the charge block stays, reads None).
	cg, ok := env.Sim.Cargos.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no cargo")
	}
	cg.Stacks = nil
	r.Sync()
	gun = r.cells[0][0].tip.View.Model()
	if len(gun.Hints) != 2 || gun.Hints[0] != "Right-click: module menu" || gun.Hints[1] != "Click: activate" {
		t.Errorf("disarmed gun hints = %q, want the menu and activate footers", gun.Hints)
	}
	if tipValue(gun, "CHARGE", "Loaded") != "None" {
		t.Errorf("charge block = %+v, want Loaded None", tipSection(gun, "CHARGE"))
	}

	// Empty slots keep their tooltip empty (the widget drops the ToolTips).
	empty := r.cells[2][2]
	if tips := empty.btn.GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("empty slot has %d tooltips, want 0", len(tips))
	}
	if got := empty.tip.View; got != nil {
		t.Error("empty slot built a tooltip panel")
	}
}

// TestRackRightClickCyclesAmmo is the command-wiring test: right-clicking
// the EMP turret opens its context menu (Show Info first and bold, then the
// charge ref, a separator, one "Load <round>" per round type in cargo, and
// Cycle charge last), and picking Cycle charge walks the four light rounds
// in cargo (item-id order) and wraps back to the first, each step enqueuing
// exactly one CmdSetAmmo. There is no Auto entry any more: the auto mode is
// gone from the design, so cycling only ever walks real types. Non-weapon
// cells never enqueue (their menu has no cycle entry at all).
func TestRackRightClickCyclesAmmo(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720) // Sync + Update registers the click handlers

	gun := r.cells[0][0] // EMP Turret, high slot 1
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}

	// Candidates: the gun's shells in cargo by item id — the sim's own bay
	// read (Clip.Options), which is what the menu and the cycle walk. The
	// class is the fitted autocannon's family: shell families split from the
	// bare calibre with the bay tranche.
	wantCands := []content.ItemID{"ammo-ap-s", "ammo-emp-s", "ammo-fusion-s", "ammo-plutonium-s"}
	if !gun.hasClip {
		t.Fatal("fixture gun declares no bay")
	}
	if got := gun.clip.Options; len(got) != len(wantCands) {
		t.Fatalf("candidates = %v, want %v", got, wantCands)
	} else {
		for i := range wantCands {
			if got[i] != wantCands[i] {
				t.Fatalf("candidates = %v, want %v", got, wantCands)
			}
		}
	}

	// The first menu: Show Info leads (bold), then the separator, the four
	// Load entries in item-id order and Cycle charge (no charge preferred
	// yet → no charge-ref row).
	rightClick(c, t, gun.btn)
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("rack right-click opened no menu")
	}
	ents := m.Entries()
	if len(ents) != 7 ||
		ents[0].Label != "Show Info" || !ents[0].Bold || ents[0].Disabled ||
		!ents[1].Sep ||
		ents[2].Label != "Load 40mm armour-piercing" ||
		ents[3].Label != "Load 40mm ion" ||
		ents[4].Label != "Load 40mm high-explosive" ||
		ents[5].Label != "Load 40mm plutonium" ||
		ents[6].Label != "Cycle charge" || ents[6].Disabled {
		t.Fatalf("rack menu entries = %+v", ents)
	}
	m.Esc()

	wantItems := []content.ItemID{"ammo-ap-s", "ammo-emp-s", "ammo-fusion-s", "ammo-plutonium-s", "ammo-ap-s"}
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
		if got := tipValue(gun.tip.View.Model(), "CHARGE", "Loaded"); got != r.ammoName(want) {
			t.Fatalf("tooltip after step %d shows charge %q, want %q", i, got, r.ammoName(want))
		}
	}
	// With a charge preferred, the menu carries its Show Info ref too
	// (Sync rebuilds the cells' declared specs — production syncs every
	// frame, so the menu always reads the current preference).
	r.ammoPref[slot] = "ammo-fusion-s"
	r.Sync()
	rightClick(c, t, gun.btn)
	if ents := env.Menu().Entries(); len(ents) != 8 || ents[1].Label != "Show Info — "+r.ammoName("ammo-fusion-s") {
		t.Fatalf("menu with a preferred charge = %+v", ents)
	}
	env.Menu().Esc()
	r.ammoPref[slot] = ""

	// The mining laser and the ammo-less rocket pod never enqueue: their
	// menus carry neither Cycle charge nor a Load entry.
	for _, idx := range []int{1, 2} {
		before := len(*cmds)
		rightClick(c, t, r.cells[0][idx].btn)
		for _, e := range env.Menu().Entries() {
			if strings.HasPrefix(e.Label, "Load ") || e.Label == "Cycle charge" {
				t.Fatalf("non-weapon cell %d menu offers %q", idx, e.Label)
			}
		}
		env.Menu().Click(env.Menu().RowRect(0).Min) // any press closes it
		if len(*cmds) != before {
			t.Fatalf("non-weapon cell %d enqueued %d commands", idx, len(*cmds)-before)
		}
	}
}

// TestRackWaitingClickOpensPicker pins the context-aware click: a waiting
// gun's left click opens its menu (the charge picker) and does NOT toggle,
// while the same cell toggles exactly as before once a round type is chosen
// — the single-click toggle contract the other tests hold.
func TestRackWaitingClickOpensPicker(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0] // EMP Turret, high slot 1
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}
	if !gun.waiting {
		t.Fatalf("fixture gun is not waiting (rounds %d, selected %q, options %v)",
			gun.clip.Rounds, gun.clip.Selection, gun.clip.Options)
	}
	if len(*cmds) != 0 {
		t.Fatalf("fixture: %d commands already queued", len(*cmds))
	}

	// A lone click on the waiting cell: the picker, not a toggle.
	spendDouble(r)
	leftClick(c, t, gun.btn)
	if len(*cmds) != 0 {
		t.Fatalf("waiting click enqueued %d commands, want none (the picker, never a toggle)", len(*cmds))
	}
	if !env.Menu().IsOpen() {
		t.Fatal("waiting click opened no menu")
	}
	// Picking a type is the whole point: one CmdSetAmmo, the sim's command.
	pressMenuEntry(t, env, "Load 40mm ion")
	if len(*cmds) != 1 {
		t.Fatalf("Load press enqueued %d commands, want 1", len(*cmds))
	}
	if cmd, ok := (*cmds)[0].(sim.CmdSetAmmo); !ok || cmd.Slot != slot || cmd.Item != "ammo-emp-s" {
		t.Fatalf("Load press = %+v (%T), want CmdSetAmmo on ammo-emp-s", (*cmds)[0], (*cmds)[0])
	}

	// The same cell with a chosen type toggles exactly once, as before.
	selectCharge(env.Sim, slot)
	*cmds = (*cmds)[:0]
	r.Sync()
	spendDouble(r)
	leftClick(c, t, gun.btn)
	if len(*cmds) != 1 {
		t.Fatalf("non-waiting click enqueued %d commands, want the single toggle", len(*cmds))
	}
	if _, ok := (*cmds)[0].(sim.CmdActivate); !ok {
		t.Fatalf("non-waiting click = %T, want sim.CmdActivate", (*cmds)[0])
	}
	if env.Menu().IsOpen() {
		t.Error("non-waiting click opened the picker")
	}
}

// TestRackLoadEntriesLoadTheChosenRound: the menu's "Load <round>" verbs are
// the picker — one per round type in cargo, each enqueuing the CmdSetAmmo
// for its own type — and the cap booster, a charge user that is no weapon,
// offers its cell the same way (with no Cycle charge: one type cannot
// cycle).
func TestRackLoadEntriesLoadTheChosenRound(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0]
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}
	before := len(*cmds)
	rightClick(c, t, gun.btn)
	pressMenuEntry(t, env, "Load 40mm high-explosive")
	if len(*cmds) != before+1 {
		t.Fatalf("Load press enqueued %d commands, want 1", len(*cmds)-before)
	}
	if cmd, ok := (*cmds)[before].(sim.CmdSetAmmo); !ok || cmd.Slot != slot || cmd.Item != "ammo-fusion-s" {
		t.Fatalf("Load press = %+v (%T), want CmdSetAmmo on ammo-fusion-s", (*cmds)[before], (*cmds)[before])
	}
	if r.ammoPref[slot] != "ammo-fusion-s" {
		t.Errorf("local ammo mirror = %q, want the loaded type", r.ammoPref[slot])
	}

	// The cap booster: fit it (the fixture flies a shield booster there) and
	// give it cells, then its cell offers the cell type and nothing else.
	fit, ok := env.Sim.Fittings.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no fitting")
	}
	fit.Mid[1] = "mod-cap-booster"
	fit.MidRT[1] = sim.ModuleRuntime{Def: "mod-cap-booster"}
	cg, ok := env.Sim.Cargos.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no cargo")
	}
	cg.Stacks = append(cg.Stacks, content.CargoStack{Item: "charge-cap-cell", Qty: 10})
	r.Sync()

	booster := r.cells[1][1]
	if !booster.hasClip || booster.waiting {
		t.Fatalf("booster bay = %+v (clip %v), want a loaded bay", booster.clip, booster.hasClip)
	}
	rightClick(c, t, booster.btn)
	ents := env.Menu().Entries()
	if len(ents) != 3 || ents[0].Label != "Show Info" || !ents[1].Sep ||
		ents[2].Label != "Load Capacitor Booster Cell" || ents[2].Disabled {
		t.Fatalf("booster menu entries = %+v", ents)
	}
	before = len(*cmds)
	pressMenuEntry(t, env, "Load Capacitor Booster Cell")
	if len(*cmds) != before+1 {
		t.Fatalf("booster Load press enqueued %d commands, want 1", len(*cmds)-before)
	}
	if cmd, ok := (*cmds)[before].(sim.CmdSetAmmo); !ok ||
		cmd.Slot != (sim.SlotRef{Class: content.SlotMid, Index: 1}) || cmd.Item != "charge-cap-cell" {
		t.Fatalf("booster Load press = %+v (%T), want CmdSetAmmo on charge-cap-cell", (*cmds)[before], (*cmds)[before])
	}
}

// TestRackWaitingBadge pins the waiting marker: a cell whose bay waits
// wears the static corner dot, a cell whose bay has a chosen type does not,
// and the badge leaves the cell's geometry, glyph and ring untouched (it is
// its own overlay, not a change to the tile).
func TestRackWaitingBadge(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0]
	if !gun.waiting {
		t.Fatalf("fixture gun is not waiting (rounds %d, options %v)", gun.clip.Rounds, gun.clip.Options)
	}
	shown := func() bool {
		return gun.badgeGfx.GetWidget().GetVisibility() == widget.Visibility_Show
	}
	if !shown() {
		t.Error("waiting cell wears no badge")
	}
	tile := gun.btn.GetWidget().Rect
	name := gun.name.GetWidget().Rect
	root := gun.root.GetWidget().Rect
	glyph := gun.btn.GraphicImage().Idle
	if badge := gun.badgeGfx.GetWidget().Rect; !badge.In(tile) {
		t.Errorf("badge rect %v escapes the tile %v", badge, tile)
	}
	if gun.badgeGfx.Image == gun.ring {
		t.Error("badge shares the ring's image: the ring's clear would wipe it")
	}
	if gun.ringOn {
		t.Error("an idle waiting cell drew a cycle ring")
	}

	// A chosen type ends the waiting state: the badge goes, nothing moves.
	selectCharge(env.Sim, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	c.Frame(t, 1280, 720)
	if gun.waiting {
		t.Error("a chosen round type still reads as waiting")
	}
	if shown() {
		t.Error("the badge is still shown with a chosen type")
	}
	if gun.btn.GetWidget().Rect != tile || gun.name.GetWidget().Rect != name ||
		gun.root.GetWidget().Rect != root {
		t.Error("the badge disturbed the cell geometry")
	}
	if gun.btn.GraphicImage().Idle != glyph {
		t.Error("the badge replaced the tile glyph")
	}

	// Modules with no bay never wear it (the miner and the rocket pod).
	for _, idx := range []int{1, 2} {
		cell := r.cells[0][idx]
		if cell.waiting || cell.badgeGfx.GetWidget().GetVisibility() == widget.Visibility_Show {
			t.Errorf("bayless cell %d wears the waiting badge", idx)
		}
	}
}

// TestRackLoadRing pins the load cycle's ring: a bay that is loading (an idle
// module's top-up — rounds in, selection not yet written, deadline ahead)
// draws the ring LIT, while a genuinely cancelled cycle on a loaded bay stays
// greyed. The two shapes read differently, and the ready marker waits for the
// load to complete.
func TestRackLoadRing(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0] // Small Autocannon, high slot 1: 3 s cycle, 40-round bay
	marker := func() bool {
		return gun.ammoGfx.GetWidget().GetVisibility() == widget.Visibility_Show
	}
	fit, _ := env.Sim.Fittings.Get(env.Sim.Player())
	rt := &fit.HighRT[0]
	now := env.Sim.Time()

	// A load in flight, half a cycle out: take-at-start rounds in the bay,
	// the selection still unwritten (that is what Loading derives from).
	rt.Active = false
	rt.Clip = content.CargoStack{Item: "ammo-emp-s", Qty: 40}
	rt.Selection = ""
	rt.CycleEnds = now + 1.5
	r.Sync()
	if !gun.ringOn || !gun.ringLit || gun.ringFrac != 0.5 {
		t.Errorf("loading ring = on:%v lit:%v frac:%v, want lit on / 0.5",
			gun.ringOn, gun.ringLit, gun.ringFrac)
	}
	if vis := gun.ringGfx.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Errorf("loading ring visibility = %d, want shown", vis)
	}
	if gun.waiting {
		t.Error("a loading bay reads as waiting")
	}
	if marker() {
		t.Error("a loading bay shows the ready marker")
	}

	// The same clock on a loaded bay with its selection written is a
	// cancellation, not a load: the ring greys out and the marker shows.
	rt.Selection = "ammo-emp-s"
	r.Sync()
	if !gun.ringOn || gun.ringLit {
		t.Errorf("cancelled ring = on:%v lit:%v, want greyed", gun.ringOn, gun.ringLit)
	}
	if !marker() {
		t.Error("a loaded cancelled bay shows no ready marker")
	}

	// Once the clock is in the past the ring goes and the bay stays armed.
	rt.CycleEnds = now
	r.Sync()
	if gun.ringOn {
		t.Error("ring after the cycle ran out, want none")
	}
	if !marker() {
		t.Error("the armed idle bay shows no ready marker")
	}

	// A dry bay (empty, a type selected, nothing loading) shows no marker.
	rt.Clip = content.CargoStack{}
	r.Sync()
	if marker() {
		t.Error("a dry bay shows the ready marker")
	}
}

// TestRackReadyMarker pins the bottom-right ammunition marker: hidden while
// the bay waits, loads, is dry or the slot is empty; shown once the bay is
// loaded and the load has completed. It rides its own overlay, so the cell
// geometry, glyph and ring never move (the waiting dot's contract, in the
// opposite corner).
func TestRackReadyMarker(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0]
	shown := func() bool {
		return gun.ammoGfx.GetWidget().GetVisibility() == widget.Visibility_Show
	}
	if !gun.waiting {
		t.Fatal("fixture gun is not waiting")
	}
	if shown() {
		t.Error("a waiting bay shows the ready marker")
	}

	// Choosing a charge starts the bay's load: rounds in, marker still held.
	selectCharge(env.Sim, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	env.Sim.Step()
	c.Frame(t, 1280, 720)
	if gun.clip.Rounds == 0 || !gun.clip.Loading {
		t.Fatalf("clip = %+v, want a load in flight", gun.clip)
	}
	if shown() {
		t.Error("a loading bay shows the ready marker")
	}
	tile := gun.btn.GetWidget().Rect
	name := gun.name.GetWidget().Rect
	root := gun.root.GetWidget().Rect
	glyph := gun.btn.GraphicImage().Idle
	badge := gun.badgeGfx.GetWidget().Rect

	// The 3 s load completes: armed, marker shown, nothing else moved.
	for i := 0; i < 180; i++ {
		env.Sim.Step()
	}
	c.Frame(t, 1280, 720)
	if gun.clip.Rounds == 0 || gun.clip.Loading {
		t.Fatalf("clip = %+v, want a completed load", gun.clip)
	}
	if !shown() {
		t.Fatal("an armed bay shows no ready marker")
	}
	if gun.btn.GetWidget().Rect != tile || gun.name.GetWidget().Rect != name ||
		gun.root.GetWidget().Rect != root {
		t.Error("the ready marker disturbed the cell geometry")
	}
	if gun.btn.GraphicImage().Idle != glyph {
		t.Error("the ready marker replaced the tile glyph")
	}
	if gun.ringOn {
		t.Error("an armed idle cell drew a cycle ring")
	}
	if mr := gun.ammoGfx.GetWidget().Rect; !mr.In(tile) {
		t.Errorf("marker rect %v escapes the tile %v", mr, tile)
	}
	if gun.ammo == gun.ring || gun.ammo == gun.badgeGfx.Image {
		t.Error("the marker shares the ring's or the badge's image")
	}
	if badges := gun.badgeGfx.GetWidget().GetVisibility(); badges == widget.Visibility_Show {
		t.Error("an armed cell still wears the waiting dot")
	}
	if gun.badgeGfx.GetWidget().Rect != badge {
		t.Error("the waiting dot moved")
	}

	// Bottom-right, by construction: the glyph's box sits wholly in the
	// tile's bottom-right quadrant and inside the overlay.
	b := gun.ammo.Bounds()
	x0, y0 := rackAmmoOrigin()
	side := theme.Px(rackAmmoIcon)
	if x0 <= b.Dx()/2 || y0 <= b.Dy()/2 {
		t.Errorf("marker origin (%d,%d) is not in the bottom-right of %v", x0, y0, b)
	}
	if x0+side > b.Dx() || y0+side > b.Dy() {
		t.Errorf("marker glyph %dx%d at (%d,%d) escapes the overlay %v", side, side, x0, y0, b)
	}

	// Bayless and empty slots never wear it (miner, rocket pod, empty low).
	for _, row := range []struct{ r, i int }{{0, 1}, {0, 2}, {2, 3}} {
		if r.cells[row.r][row.i].ammoGfx.GetWidget().GetVisibility() == widget.Visibility_Show {
			t.Errorf("cell(%d,%d) shows a ready marker", row.r, row.i)
		}
	}

	// The marker is paint-only: the armed cell still toggles on a click.
	before := len(*cmds)
	spendDouble(r)
	leftClick(c, t, gun.btn)
	if len(*cmds) != before+1 {
		t.Fatalf("armed click enqueued %d commands, want the single toggle", len(*cmds)-before)
	}
}

// TestRackTooltipClipRow pins the bay row on the cell tooltip: the bay's
// m³, the whole rounds it holds of the round it would load and the load
// cycle, the waiting wording while no type is chosen (no count at all), and
// nothing for charge-free modules.
func TestRackTooltipClipRow(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0]
	if !gun.waiting {
		t.Fatal("fixture gun is not waiting")
	}
	if got := tipValue(gun.tip.View.Model(), "CHARGE", "Clip"); got != "waiting for ammo selection" {
		t.Errorf("waiting clip row = %q, want the waiting wording", got)
	}

	// A chosen 40 mm shell: the 0.2 m³ bay holds 40 of them, one load costs
	// the gun's 3 s cycle.
	selectCharge(env.Sim, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	r.Sync()
	if got := tipValue(gun.tip.View.Model(), "CHARGE", "Clip"); got != "0.2 m³ · 40 rounds · 3.0 s" {
		t.Errorf("clip row = %q, want 0.2 m³ · 40 rounds · 3.0 s", got)
	}

	// Charge-free modules carry no clip row at all.
	for _, idx := range []int{1, 2} { // mining laser, rocket pod
		for _, sec := range r.cells[0][idx].tip.View.Model().Sections {
			for _, row := range sec.Rows {
				if row.Label == "Clip" {
					t.Errorf("cell %d carries a clip row: %+v", idx, row)
				}
			}
		}
	}
}

// TestRackDrawSmoke draws the rack in the HUD's bottom-center pair at both
// design sizes: the rack is the pair's right half, sharing its baseline.
func TestRackDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	r := NewRack(env)
	h.MountRack(r.Container())
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.Frame(t, 1280, 720)

	pair := h.pair.GetWidget().Rect
	rect := r.Container().GetWidget().Rect
	status := h.status.GetWidget().Rect
	if rect.Max.Y != pair.Max.Y || status.Max.Y != pair.Max.Y {
		t.Errorf("rack/status bottoms = %d/%d, want the pair baseline %d",
			rect.Max.Y, status.Max.Y, pair.Max.Y)
	}
	if rect.Min.X != status.Max.X+theme.Px(hudPairGap) {
		t.Errorf("rack left = %d, want %d (immediately right of the status)",
			rect.Min.X, status.Max.X+theme.Px(hudPairGap))
	}
	if pair.Max.Y < 709 || pair.Max.Y > 711 {
		t.Errorf("pair bottom = %d, want 710", pair.Max.Y)
	}
	c.Smoke(t)
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
	c.Update(t)
}

// spendDouble clears the rack's double-click arming so the next click in
// a test reads as a lone single click: the clicks here run microseconds
// apart, well inside the gesture window, where production clicks are
// human-paced.
func spendDouble(r *Rack) { r.dbl.Armed = false }

// selectCharge gives a gun's bay a chosen round type. The fixture's cargo
// carries four shell types, so a fresh bay has none selected and the cell
// sits in the WAITING state — where a left click opens the charge picker
// instead of toggling (TestRackWaitingClickOpensPicker). Every toggle
// contract below needs the ordinary, non-waiting state.
func selectCharge(s *sim.Sim, slot sim.SlotRef) {
	fit, ok := s.Fittings.Get(s.Player())
	if !ok {
		return
	}
	fit.Runtimes(slot.Class)[slot.Index].Selection = "ammo-ap-s"
}

// TestRackLeftClickToggles is the command-wiring test for defect 3:
// left-click enqueues CmdDeactivate for a running module and CmdActivate
// for an idle one (target resolved from the locks), empty slots are inert,
// and the active/inactive visual follows ModuleRuntime through Sync.
func TestRackLeftClickToggles(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	s := env.Sim
	gun := r.cells[0][0] // EMP Turret, high slot 1 (a weapon)
	miner := r.cells[0][1]
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}
	// A chosen round type is the plain toggle state (see selectCharge).
	selectCharge(s, slot)
	r.Sync()

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
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

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
	selectCharge(s, sim.SlotRef{Class: content.SlotHigh, Index: 0}) // non-waiting: a click toggles
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

// TestRackCellStacksTileAndName pins the cell column: the icon tile (with
// its cycle ring stacked over it) on top, the module's name under it — two
// disjoint, contiguous rects, all inside the cell.
func TestRackCellStacksTileAndName(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	for row := range r.cells {
		for idx, cell := range r.cells[row] {
			if cell.root.GetWidget().GetVisibility() != widget.Visibility_Show {
				continue // hidden slots have no rects to check
			}
			tile := cell.btn.GetWidget().Rect
			stack := cell.stack.GetWidget().Rect
			name := cell.name.GetWidget().Rect
			root := cell.root.GetWidget().Rect
			if tile.Dy() < rackCellH || tile.Dx() < rackCellW {
				t.Errorf("cell(%d,%d) tile = %v, want at least %d×%d",
					row, idx, tile, rackCellW, rackCellH)
			}
			if stack != tile {
				t.Errorf("cell(%d,%d) tile stack = %v, want the tile rect %v", row, idx, stack, tile)
			}
			if !cell.ringGfx.GetWidget().Rect.In(tile) {
				t.Errorf("cell(%d,%d) cycle ring %v escapes the tile %v",
					row, idx, cell.ringGfx.GetWidget().Rect, tile)
			}
			if name.Dy() <= 0 {
				t.Errorf("cell(%d,%d) name line = %v, want a measured line", row, idx, name)
			}
			if tile.Overlaps(name) {
				t.Errorf("cell(%d,%d): tile %v overlaps the name %v", row, idx, tile, name)
			}
			if stack.Max.Y != name.Min.Y {
				t.Errorf("cell(%d,%d) stack = tile %v / name %v, want contiguous",
					row, idx, tile, name)
			}
			if !stack.In(root) || !name.In(root) {
				t.Errorf("cell(%d,%d) children escape the cell %v", row, idx, root)
			}
		}
	}
}

// TestRackCellSpecData: what each cell DECLARES (its RowSpec) — the
// fitted weapon declares its module Ref, one Load verb per round type in
// cargo, the Cycle charge action and the tooltip payload (and gains the
// charge's secondary ref once one is preferred), a non-weapon declares no
// charge verbs, and an empty slot declares nothing at all (no menu, no
// tooltip).
func TestRackCellSpecData(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0] // EMP Turret: a weapon with the "small-autocannon" family
	if gun.spec.Ref == nil || *gun.spec.Ref != widgets.ModuleRef("mod-autocannon-s") {
		t.Fatalf("gun Ref = %v, want module/mod-autocannon-s", gun.spec.Ref)
	}
	if gun.spec.SecRef != nil {
		t.Errorf("gun declares a secondary ref before any charge is preferred: %v", *gun.spec.SecRef)
	}
	acts := gun.spec.Actions
	if len(acts) != 5 ||
		acts[0].Label != "Load 40mm armour-piercing" ||
		acts[1].Label != "Load 40mm ion" ||
		acts[2].Label != "Load 40mm high-explosive" ||
		acts[3].Label != "Load 40mm plutonium" ||
		acts[4].Label != "Cycle charge" {
		t.Fatalf("gun actions = %+v, want four Load entries then Cycle charge", acts)
	}
	if acts[4].Disabled {
		t.Error("Cycle charge grayed with four shell types in cargo")
	}
	if gun.spec.Tip.Key == "" || gun.spec.Tip.Build == nil {
		t.Error("gun declares no tooltip payload")
	}

	// A charge preference appears as the secondary ref after a Sync
	// (production syncs every frame, so the menu always reads it).
	slot := sim.SlotRef{Class: content.SlotHigh, Index: 0}
	r.ammoPref[slot] = "ammo-fusion-s"
	r.Sync()
	if gun.spec.SecRef == nil || *gun.spec.SecRef != widgets.ItemRef("ammo-fusion-s") {
		t.Errorf("gun SecRef = %v, want item/ammo-fusion-s", gun.spec.SecRef)
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
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	gun := r.cells[0][0]
	if len(*cmds) != 0 {
		t.Fatalf("fixture: %d commands queued", len(*cmds))
	}
	// A chosen round type keeps the cell out of the waiting state, where a
	// click would open the charge picker instead of toggling.
	selectCharge(env.Sim, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	r.Sync()

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
	if got, _ := env.InfoWindow().Ref(); got != widgets.ModuleRef("mod-autocannon-s") {
		t.Errorf("Show Info ref = %v, want module/mod-autocannon-s", got)
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

// TestRackWaitingBadgePulse: the waiting dot breathes on a frame-count
// cadence advanced in Sync (deterministic — no wall clock), swaps the
// pre-rendered phase images by pointer, and two racks fed the same Syncs
// land on the same phase.
func TestRackWaitingBadgePulse(t *testing.T) {
	env, _ := newChromeEnv(t)
	a, b := NewRack(env), NewRack(env)
	for i := 0; i < 13; i++ {
		a.Sync()
		b.Sync()
	}
	if a.badgePh != b.badgePh {
		t.Fatalf("twin racks drifted: phase %d vs %d", a.badgePh, b.badgePh)
	}
	if want := (13 / rackBadgeTicks) % rackBadgePhases; a.badgePh != want {
		t.Errorf("phase after 13 syncs = %d, want %d", a.badgePh, want)
	}
	cell := a.cells[0][0]
	phase, img := a.badgePh, cell.badgeGfx.Image
	for i := 0; i < rackBadgeTicks; i++ {
		a.Sync()
	}
	if a.badgePh == phase {
		t.Error("the pulse did not advance after one phase's ticks")
	}
	if cell.badgePhase != a.badgePh || cell.badgeGfx.Image == img {
		t.Error("the overlay did not follow the shared phase image")
	}
	// The phases are distinct images built from a ladder that really moves
	// (pixel reads are not available outside a game loop).
	if a.badge[0] == a.badge[rackBadgePhases/2] || rackBadgeAlphas[0] == rackBadgeAlphas[rackBadgePhases/2] {
		t.Error("pulse phases are identical: the dot cannot breathe")
	}
}
