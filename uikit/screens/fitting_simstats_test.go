package screens

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
)

// TestFittingDerivedStatsReadTheSim is the candidate C7 pin: the window's
// derived ship numbers are the sim's own values, read through ContentView,
// not an algebra the screen keeps beside them. Align is Motion.AlignTime,
// the recharge peak is Capacitor.RechargePeak and each EHP row is
// ShipState.EHP.
func TestFittingDerivedStatsReadTheSim(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)

	player := env.Sim.Player()
	ship, ok := env.Sim.Ships.Get(player)
	if !ok {
		t.Fatal("fixture: player has no hull")
	}
	m, ok := env.Sim.Motions.Get(player)
	if !ok {
		t.Fatal("fixture: player has no motion")
	}
	cap, ok := env.Sim.Caps.Get(player)
	if !ok {
		t.Fatal("fixture: player has no capacitor")
	}

	vals := f.statValues(player, *ship, true)
	if got, want := vals["Speed"], theme.FormatSpeed(m.MaxSpeed); got != want {
		t.Errorf("Speed = %q, want the sim's %q", got, want)
	}
	if got, want := vals["Align"], theme.GroupFloat(m.AlignTime(), 1)+" s"; got != want {
		t.Errorf("Align = %q, want the sim's align time %q", got, want)
	}
	if got, want := vals["Capacitor"], theme.GroupFloat(cap.Capacity, 0)+" GJ"; got != want {
		t.Errorf("Capacitor = %q, want the sim's %q", got, want)
	}
	for i, typ := range []content.DamageType{
		content.DmgEM, content.DmgThermal, content.DmgKinetic, content.DmgExplosive,
	} {
		if got, want := f.ehp[i].value.Label, theme.FormatInt(int64(ship.EHP(typ)+0.5)); got != want {
			t.Errorf("EHP %s row = %q, want the sim's %q", typ, got, want)
		}
	}
}
