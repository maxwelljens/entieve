package uikit

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// TestTableContentsFace pins the ContentsFace option: every row cell —
// the widget itself and the table's column measure — draws in the mono
// face, the row height follows it, the header keeps the emphasis face,
// and a table built without the option is untouched.
func TestTableContentsFace(t *testing.T) {
	env, _ := newTestEnv(t)
	mono := env.FaceMono(StepSmall)
	display := env.Face(StepSmall)
	cols := []Column{{Key: "Name", Title: "Name", Priority: 0}}

	tbl, _ := newPumpedTable(t, env, cols, ContentsFace(mono))
	plain, _ := newPumpedTable(t, env, cols)

	// The face identity: the mono table carries the mono face, the plain
	// table the display face, and the two faces are different sources.
	if *tbl.face != *mono {
		t.Errorf("mono table face = %v, want the FaceMono(StepSmall) face", *tbl.face)
	}
	if *tbl.face == *display {
		t.Error("mono table draws with the display face: the option did not take")
	}
	if *plain.face != *display {
		t.Errorf("default table face = %v, want the display face (option must be opt-in)", *plain.face)
	}

	// The column fit and the row height follow the contents face.
	probe := "12,345.67 €"
	wantW, _ := text.Measure(probe, *mono, 0)
	if got := tbl.measure(probe); got != wantW {
		t.Errorf("mono measure(%q) = %v, want %v (contents face)", probe, got, wantW)
	}
	dispW, _ := text.Measure(probe, *display, 0)
	if got := plain.measure(probe); got != dispW {
		t.Errorf("plain measure(%q) = %v, want %v (display face)", probe, got, dispW)
	}
	m := (*mono).Metrics()
	if want := math.Ceil(m.HAscent+m.HDescent) + 2*entryPadV; tbl.rowH != want {
		t.Errorf("mono rowH = %v, want %v (derived from the contents face)", tbl.rowH, want)
	}
	// Headers keep the display emphasis face, option or no option.
	bold := env.FaceBold(StepSmall)
	hw, _ := text.Measure("Name", *bold, 0)
	if got := tbl.headMeasure("Name"); got != hw {
		t.Errorf("headMeasure = %v, want %v (emphasis face)", got, hw)
	}

	// The built cell widget itself: its preferred width is the mono
	// measure of the label, not the display one.
	const label = "Młynów 1,234.56 €"
	tbl.SetRows([]TableRow{{Cells: []string{label}, Data: 0}})
	plain.SetRows([]TableRow{{Cells: []string{label}, Data: 0}})
	gotMono, _ := tbl.texts[0][0].PreferredSize()
	gotDisp, _ := plain.texts[0][0].PreferredSize()
	monoW, _ := text.Measure(label, *mono, 0)
	dispW2, _ := text.Measure(label, *display, 0)
	if want := int(math.Ceil(monoW)); gotMono != want {
		t.Errorf("mono cell width = %d, want %d (the widget draws in the contents face)", gotMono, want)
	}
	if want := int(math.Ceil(dispW2)); gotDisp != want {
		t.Errorf("plain cell width = %d, want %d (the widget draws in the display face)", gotDisp, want)
	}
	if gotMono == gotDisp {
		t.Errorf("mono and display cells both %d px wide — the probe cannot tell the faces apart", gotMono)
	}
}

// TestInventoryAndMarketRowsUseMonoFace pins the wiring: the inventory
// window's three hold tables and the market window's order book draw
// their rows in the mono contents face (IBM Plex Mono), so every
// name, quantity and price lines up column-wise.
func TestInventoryAndMarketRowsUseMonoFace(t *testing.T) {
	env, _ := newTestEnv(t)
	mono := env.FaceMono(StepSmall)

	inv := NewInventory(env)
	for name, tbl := range map[string]*Table{
		"inventory.cargo":  inv.cargo,
		"inventory.ore":    inv.ore,
		"inventory.hangar": inv.hangar,
	} {
		if *tbl.face != *mono {
			t.Errorf("%s face = %v, want the mono contents face", name, *tbl.face)
		}
	}

	m := NewMarket(env)
	if *m.tbl.face != *mono {
		t.Errorf("market order book face = %v, want the mono contents face", *m.tbl.face)
	}
}
