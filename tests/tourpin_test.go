package tests

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

// TestShowcaseFightThreeOpensOnGrid pins the recalibrated fight-3 anchor: the
// Marauder lock must fire ON GRID and resolve shortly after its command. The
// systems-scale content moved the Gniazdo landing from tick 59708 to 59986;
// the old anchor (59900) then fired mid-warp and produced "Target out of lock
// range" refusals on camera while the feature harness stayed green (its step
// windows are generous). Recalibrated 2026-10-18; lead-owned tour pin.
func TestShowcaseFightThreeOpensOnGrid(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World(showcaseWorldID)
	if !ok {
		t.Fatalf("world %s missing from content", showcaseWorldID)
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim: %v", err)
	}
	steps := loadScript(t, showcaseScript)

	// The script is the authority on when the fight opens.
	var lockCmd int64 = -1
	for _, st := range steps {
		if st.Action == "lock" && st.Target == "Marauder" {
			lockCmd = st.AtTick
		}
	}
	if lockCmd < 0 {
		t.Fatal("script has no Marauder lock step")
	}

	var tick, lockSeen int64 = 0, -1
	var refusals int
	script.RunH(s, steps, showcaseTicks, script.Hooks{
		OnTick: func(t int64, _ script.Engine) { tick = t },
		OnEvent: func(e sim.Event) {
			switch {
			case e.Kind == sim.EvTargetLocked && strings.Contains(e.Text, "Marauder") && lockSeen < 0:
				lockSeen = tick
			case strings.Contains(e.Text, "out of lock range"):
				refusals++
			}
		},
	})
	if refusals > 0 {
		t.Errorf("lock refused %dx — the fight-3 anchor fires off-grid (mid-warp?)", refusals)
	}
	// On grid and in range: the lock resolves after its command and promptly
	// (the pre-drift rhythm resolved in ~440 ticks; allow 700).
	if lockSeen < lockCmd || lockSeen > lockCmd+700 {
		t.Errorf("Marauder locked at %d, want within [%d, %d]", lockSeen, lockCmd, lockCmd+700)
	}
}
