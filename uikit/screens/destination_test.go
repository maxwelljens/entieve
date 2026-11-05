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

	env.SetDestination("sys-solace")
	h.Sync()
	if !h.destRead.show || h.destRead.name != "Struvok" || h.destRead.dest != "" {
		t.Fatalf("in-system dest = {%v %q %q}, want the star Struvok with no final system",
			h.destRead.show, h.destRead.name, h.destRead.dest)
	}

	env.SetDestination("")
	h.Sync()
	if h.destRead.show {
		t.Error("the readout stayed up after the destination cleared")
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
		if info.Cel == gateCel {
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
