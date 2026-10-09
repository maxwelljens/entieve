package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// readFile reads a file relative to the repository root (tests run with their
// working directory inside the tests package).
func readFile(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join("..", rel))
}

// probe is one sampled observation of the player's state during a run.
type probe struct {
	Tick              int64
	Pos               core.Vec2
	Speed             float64
	Cap, CapMax       float64
	Shield, ShieldMax float64
	Armor, ArmorMax   float64
	Structure         float64
}

// evidence bundles everything feature rules may inspect.
type evidence struct {
	Events     []sim.Event
	Start      *sim.Save
	End        *sim.Save
	Probes     []probe
	Unresolved []string

	// Programmatic checks performed by the showcase harness itself.
	SaveRestoreOK bool
	DeterminismOK bool
}

// playerOf returns the saved player entity, or nil.
func playerOf(s *sim.Save) *sim.SavedEntity {
	for i := range s.Entities {
		e := &s.Entities[i]
		if e.Pilot != nil && e.Pilot.Player {
			return e
		}
	}
	return nil
}

// itemQty counts units of an item in the player's cargo, ore hold and hangar.
func itemQty(s *sim.Save, id content.ItemID) int {
	total := 0
	if p := playerOf(s); p != nil && p.Cargo != nil {
		for _, st := range append(append([]content.CargoStack{}, p.Cargo.Stacks...), p.Cargo.Ore...) {
			if st.Item == id {
				total += st.Qty
			}
		}
	}
	for _, st := range s.Hangar.Items {
		if st.Item == id {
			total += st.Qty
		}
	}
	return total
}

// itemQtyDropped reports how many units of id the player lost between saves.
func (e *evidence) itemQtyDropped(id content.ItemID) int {
	return itemQty(e.Start, id) - itemQty(e.End, id)
}

// eventRe reports whether any event text matches the pattern (case-insensitive).
func (e *evidence) eventRe(pattern string) bool {
	re := regexp.MustCompile(`(?i)` + pattern)
	for _, ev := range e.Events {
		if re.MatchString(ev.Text) {
			return true
		}
	}
	return false
}

// kindSeen reports whether any event of the given kind (optionally with a
// phase) was emitted.
func (e *evidence) kindSeen(kind sim.EventKind, phase string) bool {
	for _, ev := range e.Events {
		if ev.Kind == kind && (phase == "" || ev.Phase == phase) {
			return true
		}
	}
	return false
}

// playerFittingKinds maps fitted module IDs to their content kinds.
func (e *evidence) playerFittingKinds(c *content.Content) map[string]bool {
	kinds := map[string]bool{}
	p := playerOf(e.End)
	if p == nil || p.Fitting == nil {
		return kinds
	}
	for _, id := range append(append([]content.ModuleID{}, p.Fitting.High...), append(p.Fitting.Mid, p.Fitting.Low...)...) {
		if def, ok := c.Module(id); ok {
			kinds[def.Kind] = true
		}
	}
	return kinds
}

// minCapFrac / maxCapFrac summarize capacitor observations.
func (e *evidence) minCapFrac() float64 {
	min := 1.0
	for _, p := range e.Probes {
		if p.CapMax > 0 {
			if f := p.Cap / p.CapMax; f < min {
				min = f
			}
		}
	}
	return min
}

// capRoseLater reports whether the capacitor was observed draining below
// drainBelow and later refilling above refillAbove.
func (e *evidence) capDrainThenRecover(drainBelow, refillAbove float64) bool {
	sawDrain := false
	for _, p := range e.Probes {
		if p.CapMax <= 0 {
			continue
		}
		f := p.Cap / p.CapMax
		if f < drainBelow {
			sawDrain = true
		}
		if sawDrain && f > refillAbove {
			return true
		}
	}
	return false
}

// layerRose reports whether a shield/armor value was observed rising between
// probes by at least delta (repair/boost evidence) after having been lower.
func (e *evidence) layerRose(which string, delta float64) bool {
	prev := -1.0
	for _, p := range e.Probes {
		var v float64
		switch which {
		case "shield":
			v = p.Shield
		case "armor":
			v = p.Armor
		}
		if prev >= 0 && v > prev+delta {
			return true
		}
		prev = v
	}
	return false
}

// movedSublight reports position changes between consecutive probes that are
// too small/short to be a warp (warp jumps hundreds of km per second).
func (e *evidence) movedSublight(minMeters, maxMeters float64) bool {
	for i := 1; i < len(e.Probes); i++ {
		d := e.Probes[i].Pos.Dist(e.Probes[i-1].Pos)
		if d >= minMeters && d <= maxMeters {
			return true
		}
	}
	return false
}

// walletDelta is credits gained (bounty, sales) minus spent.
func (e *evidence) walletDelta() float64 {
	return e.End.Wallet - e.Start.Wallet
}

// allAmmoTypesFired reports whether events mention every light charge name.
func (e *evidence) allAmmoTypesFired() bool {
	for _, pat := range []string{`emp`, `fusion`, `\bap\b`, `plutonium`} {
		if !e.eventRe(pat) {
			return false
		}
	}
	return true
}

// summarize renders a compact failure report.
func (e *evidence) summarize() string {
	var b strings.Builder
	b.WriteString("events:\n")
	for _, ev := range e.Events {
		b.WriteString("  [" + string(ev.Kind) + "] " + ev.Text + "\n")
	}
	if len(e.Unresolved) > 0 {
		b.WriteString("unresolved script steps:\n")
		for _, s := range e.Unresolved {
			b.WriteString("  " + s + "\n")
		}
	}
	return b.String()
}
