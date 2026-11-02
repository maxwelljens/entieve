package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// TestStandingGateFlipsWithTheSim is the drift pin for candidate C3: the
// contact row and the dialog's disabled Accept both call the sim's
// cv.MeetsStanding, so the screen and the accept refusal must flip at the
// same standing, epsilon included. Driving the pilot across a real offer's
// minimum from both sides is the bug class the duplicated comparison
// invited.
func TestStandingGateFlipsWithTheSim(t *testing.T) {
	env, _ := newChromeEnv(t)
	const at = content.CelID("cel-penumbra-port")
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: at})

	var offer sim.Offer
	for _, o := range env.Sim.ContactOffers(at) {
		if o.MinStanding > 0 {
			offer = o
			break
		}
	}
	if offer.Def == nil {
		t.Fatal("fixture: no standing-gated offer at the port")
	}
	min := offer.MinStanding
	want := gateText(min)
	if want != "Requires standing 0.20" {
		t.Fatalf("the gated offer states %q, want the sim's own sentence", want)
	}

	setStanding := func(v float64) {
		t.Helper()
		sv, err := env.Sim.Snapshot()
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		sv.Standing = v
		if err := env.Sim.Restore(sv); err != nil {
			t.Fatalf("Restore: %v", err)
		}
		env.Sim.DrainEvents()
	}
	// accept runs the real accept command and returns the standing refusal
	// the sim emitted, if any.
	accept := func() string {
		t.Helper()
		env.Sim.Queue(sim.CmdAcceptMission{ID: offer.Def.ID})
		env.Sim.Step()
		for _, e := range env.Sim.DrainEvents() {
			if strings.Contains(e.Text, "Requires standing") {
				return e.Text
			}
		}
		return ""
	}

	// Just short of the gate: both sides refuse, in the same words.
	setStanding(min - 2e-9)
	if env.MeetsStanding(min) {
		t.Fatal("the view clears a standing the sim refuses")
	}
	if got := accept(); !strings.HasSuffix(got, want) {
		t.Fatalf("refusal = %q, the row/dialog state %q", got, want)
	}

	// Inside the sim's epsilon: both sides clear.
	setStanding(min - 5e-10)
	if !env.MeetsStanding(min) {
		t.Fatal("the view refuses a standing the sim accepts")
	}
	if got := accept(); got != "" {
		t.Fatalf("accept refused at standing %v: %q", min-5e-10, got)
	}
}
