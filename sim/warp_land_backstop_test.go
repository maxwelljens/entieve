package sim

import (
	"strconv"
	"testing"

	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// F1 — the align phase has no landing guard of its own: it only transitions
// to "warp" on the 75%-speed + facing gate, so a warp whose landing point is
// nearly coincident with the ship (NPC warpToPoint callers, or any future
// caller outside the 150 km player rule) can never satisfy the gate — the
// ship reaches the point before it is up to speed and oscillates around it —
// and the drive sat in "align" forever with CmdStop and re-warp refused.
// The align backstop lands the drive (exactly like the warp phase) the
// moment the ship is inside warpLandRange of the landing point.
// ---------------------------------------------------------------------------

// alignBackstopCases are the sub-kilometre landing distances the review
// demanded: coincident (0 m), well inside the align run-up, and straddling
// the ~0.28125*MaxSpeed*alignTime run-up distance (~337 m for the fixture
// sloop) below which the gate is unreachable.
var alignBackstopCases = []float64{0, 100, 400, 490, 510}

// alignBackstopBudget is the tick ceiling every case must land inside; a
// stuck alignment must not survive it.
const alignBackstopBudget = 1800

// phaseOf returns the drive phase of e, or "" when the drive is released.
func phaseOf(s *Sim, e ecs.Entity) string {
	if wd, ok := s.Warps.Get(e); ok {
		return wd.Phase
	}
	return ""
}

// runAlignBackstop steps the sim until e's warp drive releases, returning
// the release tick (-1 = never: still engaged at the budget) and whether the
// release came with a "Warp complete" landing event.
func runAlignBackstop(s *Sim, e ecs.Entity) (releasedTick int, landed bool) {
	for i := 0; i < alignBackstopBudget; i++ {
		s.Step()
		if findEvent(drain(s), EvWarp, "Warp complete") != nil {
			landed = true
		}
		if !s.Warps.Has(e) {
			return int(s.Ticks()), landed
		}
	}
	return -1, landed
}

// TestPlayerAlignBackstopLandsShortWarps pins the player-side backstop for
// every sub-kilometre landing point: the drive always lands within a bounded
// tick count, never survives the budget in "align", and snaps exactly onto
// the landing point.
func TestPlayerAlignBackstopLandsShortWarps(t *testing.T) {
	for _, d := range alignBackstopCases {
		d := d
		t.Run(strconv.FormatFloat(d, 'f', -1, 64)+"m", func(t *testing.T) {
			s := newTestSim(t)
			start := v(2e7, 2e7) // empty space: no celestial, no NPC
			setPlayerPos(s, start)
			land := start.Add(v(d, 0))
			s.Intents.Set(s.player, Intent{Kind: IntentStop})
			// Direct warp-state setup: CmdWarpTo refuses inside
			// WarpMinRange, so this state is reached only through a
			// save/restore of an engaged drive or a non-player caller.
			s.Warps.Set(s.player, WarpDrive{Phase: "align", To: land})

			released, landed := runAlignBackstop(s, s.player)
			if released < 0 {
				t.Fatalf("drive still in %q after %d ticks: align deadlock",
					phaseOf(s, s.player), alignBackstopBudget)
			}
			if !landed {
				t.Fatalf("drive released on tick %d without a landing event", released)
			}
			if released > 10 {
				t.Fatalf("released on tick %d, want the backstop within ~1 tick", released)
			}
			if got := playerPos(s); got.Dist(land) > 1e-9 {
				t.Fatalf("landed at %v, want the landing point %v", got, land)
			}
		})
	}
}

// TestNpcAlignBackstopLandsShortWarps is the same guarantee through the
// non-player caller (warpToPoint), whose NPC callers only guard the target
// distance — a landing point inside the tolerance must still resolve.
func TestNpcAlignBackstopLandsShortWarps(t *testing.T) {
	for _, d := range alignBackstopCases {
		d := d
		t.Run(strconv.FormatFloat(d, 'f', -1, 64)+"m", func(t *testing.T) {
			s := newTestSim(t)
			start := v(2e7, 2e7)
			// Keep the player far away: the guard trigger (d) must not pull
			// the NPC off its warp and onto the player.
			setPlayerPos(s, v(0, 0))
			npc := s.spawnNpc("npc-rat-scuttler", start, 50)
			if npc.IsZero() {
				t.Fatal("spawnNpc failed")
			}
			tr, _ := s.Transforms.Get(npc)
			tr.Pos = start // ignore the spawn scatter: the point is start+d
			land := start.Add(v(d, 0))
			if !s.warpToPoint(npc, land) {
				t.Fatal("warpToPoint refused to engage")
			}

			released, landed := runAlignBackstop(s, npc)
			if released < 0 {
				t.Fatalf("NPC drive still in %q after %d ticks: align deadlock",
					phaseOf(s, npc), alignBackstopBudget)
			}
			if !landed {
				t.Fatalf("NPC drive released on tick %d without a landing event", released)
			}
			if released > 10 {
				t.Fatalf("released on tick %d, want the backstop within ~1 tick", released)
			}
			if got, _ := s.posOf(npc); got.Dist(land) > 1e-9 {
				t.Fatalf("landed at %v, want the landing point %v", got, land)
			}
		})
	}
}
