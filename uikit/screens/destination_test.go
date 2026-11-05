package screens

import (
	"image/color"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestHudDestinationReadout pins the top readout: hidden with no planned
// system, the route's next gate with the final system while it is further,
// the current system's star when the destination is here, and hidden again
// when the plan clears.
func TestHudDestinationReadout(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.SetDestination(env.Destination)
	h.Sync()
	if h.destRead.show {
		t.Fatal("the readout is up with no destination")
	}
	if vis := h.destRow.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("no destination leaves the row visible (%d)", vis)
	}

	env.SetDestination("sys-penumbra")
	h.Sync()
	if !h.destRead.show {
		t.Fatal("the readout stayed hidden with a destination set")
	}
	if h.destRead.name != "Gate (Kvonice)" {
		t.Errorf("next gate = %q, want Gate (Kvonice)", h.destRead.name)
	}
	if h.destRead.dest != "Kvonice" {
		t.Errorf("final system = %q, want Kvonice", h.destRead.dest)
	}
	if h.destRead.dist == "" {
		t.Error("the readout carries no distance")
	}
	if vis := h.destRow.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Errorf("a set destination leaves the row hidden (%d)", vis)
	}

	env.SetDestination("sys-solace") // the current system: arrival
	h.Sync()
	if !h.destRead.show || !h.destRead.reached || h.destRead.name != destReachedLabel {
		t.Fatalf("arrival readout = {show:%v reached:%v %q}, want the arrival line",
			h.destRead.show, h.destRead.reached, h.destRead.name)
	}

	env.SetDestination("")
	h.Sync()
	if h.destRead.show {
		t.Error("the readout stayed up after the destination cleared")
	}
	if h.destReached != 0 {
		t.Errorf("arrival clock = %d with no destination, want it reset", h.destReached)
	}
}

// TestHudDestinationReachedFadesOut: the arrival line holds the panel for
// hudDestFade frames and then the row hides (the destination itself stays
// set, so the Starmap keeps its route); leaving the system resets the
// clock, so a later return says it again.
func TestHudDestinationReachedFadesOut(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.SetDestination(env.Destination)
	env.SetDestination("sys-solace") // already in solace: arrival
	h.Sync()
	if !h.destRead.reached || h.destRead.show && h.destRead.fade >= 1 {
		t.Fatalf("arrival = {reached:%v show:%v fade:%v}, want the line up below full alpha",
			h.destRead.reached, h.destRead.show, h.destRead.fade)
	}

	for i := 0; i < hudDestFade+2; i++ {
		h.Sync()
	}
	if h.destRead.show {
		t.Errorf("the arrival line is still up after %d frames (fade %v)", hudDestFade, h.destRead.fade)
	}
	if vis := h.destRow.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("the row did not hide after the fade (%d)", vis)
	}

	// Leaving the system resets the clock and brings the route readout back.
	env.SetDestination("sys-penumbra")
	h.Sync()
	if h.destReached != 0 {
		t.Errorf("arrival clock = %d outside the destination system, want reset", h.destReached)
	}
	if h.destRead.reached || !h.destRead.show {
		t.Fatalf("outside the system = {reached:%v show:%v}, want the route readout",
			h.destRead.reached, h.destRead.show)
	}

	// Returning says it again.
	env.SetDestination("sys-solace")
	h.Sync()
	if !h.destRead.reached || !h.destRead.show {
		t.Fatalf("on return = {reached:%v show:%v}, want the arrival line again",
			h.destRead.reached, h.destRead.show)
	}
}

// TestHudDestinationArrowFollowsHeading pins the arrow's bearing against the
// ship's own heading: a target straight ahead is 0, the same target behind
// is pi. The numbers come from the live transforms, not a snapshot.
func TestHudDestinationArrowFollowsHeading(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.SetDestination(env.Destination)
	player := env.Sim.Player()
	gateCel, ok := gateTo(env, "sys-solace", "sys-penumbra")
	if !ok {
		t.Fatal("fixture: Solace has no gate to Penumbra")
	}
	var gate sim.Info
	for _, info := range env.Sim.Grid() {
		if info.Kind == sim.KindGate && info.Cel == gateCel {
			gate = info
		}
	}
	if gate.Entity == 0 {
		t.Fatal("fixture: the gate is not on the grid")
	}
	env.Sim.Transforms.Set(player, sim.Transform{Pos: core.V(0, 0)})
	m, ok := env.Sim.Motions.Get(player)
	if !ok {
		t.Fatal("fixture: the player has no motion")
	}
	m.Vel = core.V(100, 0) // heading screen-right
	env.SetDestination("sys-penumbra")

	env.Sim.Transforms.Set(gate.Entity, sim.Transform{Pos: core.V(1000, 0)})
	h.Sync()
	if !h.destRead.arrow || math.Abs(h.destRead.angle) > 0.01 {
		t.Errorf("gate ahead: arrow=%v angle=%v, want 0", h.destRead.arrow, h.destRead.angle)
	}

	env.Sim.Transforms.Set(gate.Entity, sim.Transform{Pos: core.V(-1000, 0)})
	h.Sync()
	if !h.destRead.arrow || math.Abs(math.Abs(h.destRead.angle)-math.Pi) > 0.01 {
		t.Errorf("gate behind: arrow=%v angle=%v, want pi", h.destRead.arrow, h.destRead.angle)
	}
}

// TestOverviewDestinationGateTint: only the route's next gate wears the
// route accent on both channels, and the kind's Colour switch still zeroes
// it (the pilot and hostile tints' rule).
func TestOverviewDestinationGateTint(t *testing.T) {
	k, _ := newTestKit(t)
	k.Env().SetDestination("sys-penumbra")
	pumpKit(t, k, 1024, 700)
	nextCel, ok := gateTo(k.cv, "sys-solace", "sys-penumbra")
	if !ok {
		t.Fatal("fixture: Solace has no gate to Penumbra")
	}
	pal := k.th.Pal()
	seen := false
	for _, r := range k.ov.tbl.RowsForTest() {
		row, ok := r.Data.(*ovRow)
		if !ok || row.info.Kind != sim.KindGate {
			continue
		}
		if row.info.Cel == nextCel {
			seen = true
			if want := widgets.WithA(pal.StatMobility, 60); r.Tint != want || r.IconTint != pal.StatMobility {
				t.Errorf("next gate tint = %v / %v, want %v / %v",
					r.Tint, r.IconTint, want, pal.StatMobility)
			}
			// Owner ruling: the destination highlight must never be the
			// selection colour (Accent tints the selected row).
			if r.Tint == widgets.WithA(pal.Accent, 60) || r.IconTint == pal.Accent {
				t.Error("the next gate wears Accent, which is selection")
			}
			continue
		}
		if r.Tint != (color.RGBA{}) || r.IconTint != (color.RGBA{}) {
			t.Errorf("non-route gate %q wears a tint (%v / %v)", row.info.Name, r.Tint, r.IconTint)
		}
	}
	if !seen {
		t.Fatal("fixture: the next gate is not in the overview")
	}

	k.SetOverviewFilter(OvFilter{sim.KindGate: {Plain: true}})
	pumpKit(t, k, 1024, 700)
	for _, r := range k.ov.tbl.RowsForTest() {
		row, ok := r.Data.(*ovRow)
		if ok && row.info.Kind == sim.KindGate && (r.Tint != (color.RGBA{}) || r.IconTint != (color.RGBA{})) {
			t.Errorf("colour off still tints gate %q (%v / %v)", row.info.Name, r.Tint, r.IconTint)
		}
	}
}

// TestMapViewWritesTheSessionDestination: the map is the one writer, and a
// pick lands in the session home the HUD and overview read.
func TestMapViewWritesTheSessionDestination(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env, nil, nil)
	m.SetDestination(env.SetDestination)
	m.pickSystem("sys-glusza", "sys-solace") // selects
	m.pickSystem("sys-glusza", "sys-solace") // sets
	if got := env.Destination(); got != content.SystemID("sys-glusza") {
		t.Fatalf("session destination = %q, want sys-glusza", got)
	}
	m.pickSystem("sys-glusza", "sys-solace") // clears
	if got := env.Destination(); got != "" {
		t.Errorf("cleared destination = %q, want none", got)
	}
}

// TestDestinationStaysSessionOnly: writing the plan moves no sim state, so
// the session is its only home (never saved, never a command).
func TestDestinationStaysSessionOnly(t *testing.T) {
	env, _ := newChromeEnv(t)
	before := env.Sim.Hash()
	env.SetDestination("sys-penumbra")
	if after := env.Sim.Hash(); after != before {
		t.Errorf("setting a destination moved the sim hash: %x -> %x", before, after)
	}
	env.SetDestination("")
	if after := env.Sim.Hash(); after != before {
		t.Errorf("clearing a destination moved the sim hash: %x -> %x", before, after)
	}
}

// TestHudDestinationReadoutNamesTheGateNotAShipOnIt: Info.Cel is the
// NEAREST celestial for loose grid objects (sim/api.go), so a ship parked
// on the gate carries the gate's id too, and Grid is handle-ordered with
// the player's own hull near the front. Owner report: "When warping to the
// gate, it shows my ship name at the top (the navigation guide) instead of
// the gate." The lookup must ask for the gate, not for anything carrying
// its id.
func TestHudDestinationReadoutNamesTheGateNotAShipOnIt(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.SetDestination(env.Destination)
	player := env.Sim.Player()
	gateCel, ok := gateTo(env, "sys-solace", "sys-penumbra")
	if !ok {
		t.Fatal("fixture: Solace has no gate to Penumbra")
	}
	var gate sim.Info
	for _, info := range env.Sim.Grid() {
		if info.Kind == sim.KindGate && info.Cel == gateCel {
			gate = info
		}
	}
	if gate.Entity == 0 {
		t.Fatal("fixture: the gate is not on the grid")
	}
	// Park the player on the gate: from then on its own Info carries the
	// gate's Cel, which is exactly what used to hijack the name.
	env.Sim.Transforms.Set(player, sim.Transform{Pos: gate.Pos})
	env.SetDestination("sys-penumbra")
	h.Sync()
	if !h.destRead.show {
		t.Fatal("the readout is down with a destination set")
	}
	if h.destRead.name != gate.Name {
		t.Errorf("readout names %q, want the gate %q (a ship on the gate carries its Cel)",
			h.destRead.name, gate.Name)
	}
}

// TestOverviewTintsOnlyTheGateNotWhatSitsOnIt: the same nearest-celestial
// rule in the overview. Owner report: "It also highlights ships and other
// entities at the gate." Every loose entity parked on the next gate must
// stay untinted while the gate row itself wears the plan's hue.
func TestOverviewTintsOnlyTheGateNotWhatSitsOnIt(t *testing.T) {
	k, _ := newTestKit(t)
	k.Env().SetDestination("sys-penumbra")
	pumpKit(t, k, 1024, 700)
	nextCel, ok := gateTo(k.cv, "sys-solace", "sys-penumbra")
	if !ok {
		t.Fatal("fixture: Solace has no gate to Penumbra")
	}
	var gate sim.Info
	var impostors []sim.Info
	player := k.Sim.Player()
	for _, info := range k.Sim.Grid() {
		switch {
		case info.Entity == player:
		case info.Kind == sim.KindGate && info.Cel == nextCel:
			gate = info
		case looseKind(info.Kind) && !info.Hostile:
			// A neutral loose object (ship, rock, wreck): the class the
			// nearest-celestial rule gives the gate's id. Celestials are
			// excluded on purpose: their Cel is their own id by rule.
			impostors = append(impostors, info)
		}
	}
	if gate.Entity == 0 {
		t.Fatal("fixture: the gate is not on the grid")
	}
	if len(impostors) == 0 {
		t.Fatal("fixture: no neutral entity to park on the gate")
	}
	// Bring the gate to the player and park the impostors exactly on it:
	// left at the gate's authored distance the rows would be invisible
	// (Visible = landmark or within sensor range), and an entity that is
	// not nearest the gate never carries its id either.
	me, ok := k.Sim.Info(player)
	if !ok {
		t.Fatal("fixture: the player is not on the grid")
	}
	at := me.Pos.Add(core.V(400, 0))
	k.Sim.Transforms.Set(gate.Entity, sim.Transform{Pos: at})
	for _, info := range impostors {
		k.Sim.Transforms.Set(info.Entity, sim.Transform{Pos: at})
	}
	// The fixture must reproduce the owner's bug: every parked entity
	// carries the gate's id through the nearest-celestial rule.
	for _, info := range impostors {
		now, ok := k.Sim.Info(info.Entity)
		if !ok || now.Cel != nextCel {
			t.Fatalf("fixture: %q is not nearest the gate (Cel %q, want %q)", info.Name, now.Cel, nextCel)
		}
	}
	pumpKit(t, k, 1024, 700)

	pal := k.th.Pal()
	teal := widgets.WithA(pal.StatMobility, 60)
	seenGate, seenImpostor := false, false
	for _, r := range k.ov.tbl.RowsForTest() {
		row, ok := r.Data.(*ovRow)
		if !ok {
			continue
		}
		switch {
		case row.info.Entity == gate.Entity:
			seenGate = true
			if r.Tint != teal || r.IconTint != pal.StatMobility {
				t.Errorf("gate tint = %v / %v, want the plan's teal %v / %v",
					r.Tint, r.IconTint, teal, pal.StatMobility)
			}
		case row.info.Entity != player && !row.info.Hostile && row.info.Kind != sim.KindGate:
			seenImpostor = true
			if r.Tint == teal || r.IconTint == pal.StatMobility {
				t.Errorf("%q (%s, on the gate) wears the gate's teal (%v / %v)",
					row.info.Name, row.info.Kind, r.Tint, r.IconTint)
			}
		}
	}
	if !seenGate {
		t.Fatal("fixture: the gate row is not in the overview")
	}
	if !seenImpostor {
		t.Log("fixture: no parked entity row reached the overview (all invisible?)")
	}
}

// looseKind reports whether an Info is a loose grid object: the class the
// nearest-celestial rule hands a celestial id to (sim/api.go). Celestials
// keep their own id and are not impostors.
func looseKind(kind string) bool {
	switch kind {
	case sim.KindShip, sim.KindAsteroid, sim.KindWreck:
		return true
	}
	return false
}
