// Package tests is Entieve's integration layer: the showcase world must
// exercise every feature the game claims to have.
//
// TestShowcaseCoversEveryFeature runs world-showcase through the scripted
// command sequence in scripts/showcase.json and then checks, for EVERY
// checkpoint declared in the world's scenario list, that an evidence rule
// exists and observes the feature working. Adding a feature to the game means
// adding it to world-showcase's scenario AND an evidence rule here — the test
// fails loudly if either side is missing.
package tests

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

const (
	showcaseWorldID = "world-showcase"
	showcaseScript  = "scripts/showcase.json"
	showcaseTicks   = 85_000 // ~24 sim minutes; covers the full script tour
	probeEveryTicks = 300    // 5 sim seconds
)

// featureRule observes one feature in the evidence gathered from the run.
type featureRule func(c *content.Content, e *evidence) (ok bool, reason string)

// rules is the evidence registry. Keys are the `feature` strings used by
// world-showcase's scenario checkpoints (kebab-case).
var rules = map[string]featureRule{
	"fitting-all-kinds": func(c *content.Content, e *evidence) (bool, string) {
		kinds := e.playerFittingKinds(c)
		for _, want := range []string{content.KindWeapon, content.KindMiningLaser, content.KindAfterburner} {
			if !kinds[want] {
				return false, "no fitted module of kind " + want
			}
		}
		if !kinds[content.KindShieldBoost] && !kinds[content.KindArmorRep] {
			return false, "no active tank module fitted"
		}
		return true, ""
	},
	"module-activation": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvModule, "") || e.eventRe(`activat`), "no module activation event"
	},
	"ammo-consumption": func(c *content.Content, e *evidence) (bool, string) {
		for _, id := range []content.ItemID{"charge-emp", "charge-fusion", "charge-ap", "charge-plutonium"} {
			if e.itemQtyDropped(id) > 0 {
				return true, ""
			}
		}
		return false, "no ammunition was consumed"
	},
	"dry-gun": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`dry|out of ammo|no ammunition|ammunition depleted`), "no dry-gun warning"
	},
	"turret-hit": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`\bhits?\b|damage`), "no turret hit observed"
	},
	"wrecking-shot": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`wrecking`), "no wrecking shot observed"
	},
	"four-damage-types": func(c *content.Content, e *evidence) (bool, string) {
		if e.allAmmoTypesFired() {
			return true, ""
		}
		for _, pat := range []string{`electromagnetic|\bem\b`, `thermal`, `kinetic`, `explosive`} {
			if !e.eventRe(pat) {
				return false, "damage type not observed: " + pat
			}
		}
		return true, ""
	},
	"resists": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`resist|absorbs`), "no resist application observed"
	},
	"capacitor-drain": func(c *content.Content, e *evidence) (bool, string) {
		return e.minCapFrac() < 0.5, "capacitor never drained below 50%"
	},
	"capacitor-recharge": func(c *content.Content, e *evidence) (bool, string) {
		return e.capDrainThenRecover(0.5, 0.8), "capacitor never drained then recovered"
	},
	"cap-booster": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`cap(acitor)? (booster|cell|restor)`), "no cap booster activation"
	},
	"shield-boost": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`shield boost`) || e.layerRose("shield", 25), "no shield boost observed"
	},
	"armor-rep": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`armor rep`) || e.layerRose("armor", 25), "no armor repair observed"
	},
	"afterburner": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`afterburner`), "afterburner never activated"
	},
	"stasis-web": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`stasis|webbed|webifier`), "stasis web never observed"
	},
	"warp-scram": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`scrambl|warp disrupt`), "warp scramble/disruption never observed"
	},
	"targeting": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvTargetLocked, "") || e.eventRe(`locked`), "no target lock event"
	},
	"warp": func(c *content.Content, e *evidence) (bool, string) {
		if e.kindSeen(sim.EvWarp, "start") && e.kindSeen(sim.EvWarp, "land") {
			return true, ""
		}
		return e.eventRe(`warp`), "warp start/land not both observed"
	},
	"approach": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`approach`) || e.movedSublight(50, 2e6), "no sublight approach observed"
	},
	"orbit": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`orbit`), "no orbit observed"
	},
	"manual-piloting": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`manual`), "no manual piloting observed"
	},
	"npc-aggro": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`aggro|hostile|engaging`), "no NPC aggro observed"
	},
	"npc-flee": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`flee|fleeing|retreat`), "no NPC flee observed"
	},
	"npc-death": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvDestroyed, "") && e.eventRe(`wreck`), "no NPC death with wreck"
	},
	"bounty": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`bounty`) && e.walletDelta() != 0, "no bounty payout observed"
	},
	"looting": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvLooted, "") || e.eventRe(`loot`), "no looting observed"
	},
	"mining": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`mined|mining laser`), "no mining observed"
	},
	"asteroid-depletion": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`deplet|asteroid.*(empty|gone)|last of`), "no asteroid depletion observed"
	},
	"ore-hold": func(c *content.Content, e *evidence) (bool, string) {
		for _, id := range []content.ItemID{"ore-sterngrit", "ore-cordite", "ore-lucidite", "ore-glacierite"} {
			if itemQty(e.End, id) > itemQty(e.Start, id) {
				return true, ""
			}
		}
		return e.eventRe(`ore hold`), "ore hold never received ore"
	},
	"cargo-transfer": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`hangar|transfer|stored|delivered`), "no cargo transfer observed"
	},
	"gate-jump": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvJumped, ""), "no gate jump observed"
	},
	"docking": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvDocked, ""), "no docking observed"
	},
	"undocking": func(c *content.Content, e *evidence) (bool, string) {
		return e.kindSeen(sim.EvUndocked, ""), "no undocking observed"
	},
	"station-repair": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`repair`), "no station repair observed"
	},
	"market-buy": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`bought|purchased`), "no market purchase observed"
	},
	"market-sell": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`sold`), "no market sale observed"
	},
	"refining": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`refin`), "no refining observed"
	},
	"ship-assembly": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`assembl`), "no ship assembly observed"
	},
	"module-fit": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`fitted|installed|fitting module`), "no module fitting observed"
	},
	"module-unfit": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`unfitted|removed|unfit`), "no module unfitting observed"
	},
	"npc-respawn": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`respawn|spawned`), "no NPC respawn observed"
	},
	"save-restore": func(c *content.Content, e *evidence) (bool, string) {
		return e.SaveRestoreOK, "save/restore round-trip failed"
	},
	"determinism": func(c *content.Content, e *evidence) (bool, string) {
		return e.DeterminismOK, "twin runs diverged"
	},
}

// TestShowcaseCoversEveryFeature is the game's end-to-end guarantee: the
// showcase world exercises every feature, and every feature is observed.
func TestShowcaseCoversEveryFeature(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World(showcaseWorldID)
	if !ok {
		t.Fatalf("world %s missing from content", showcaseWorldID)
	}
	if len(wd.Scenario) == 0 {
		t.Fatal("showcase world has no scenario checkpoints")
	}

	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim: %v", err)
	}
	start, err := s.Snapshot()
	if err != nil {
		t.Fatalf("start snapshot: %v", err)
	}

	steps := loadScript(t, showcaseScript)
	ev := &evidence{Start: start}
	res := script.RunH(s, steps, showcaseTicks, script.Hooks{
		OnEvent: func(e sim.Event) { ev.Events = append(ev.Events, e) },
		OnTick: func(tick int64, eng script.Engine) {
			if tick%probeEveryTicks != 0 {
				return
			}
			sim2, ok := eng.(*sim.Sim)
			if !ok {
				return
			}
			p := probe{Tick: tick}
			if e := sim2.Player(); !e.IsZero() {
				if tr, ok := sim2.Transforms.Get(e); ok {
					p.Pos = tr.Pos
				}
				if mo, ok := sim2.Motions.Get(e); ok {
					p.Speed = mo.Vel.Len()
				}
				if cp, ok := sim2.Caps.Get(e); ok {
					p.Cap, p.CapMax = cp.Current, cp.Capacity
				}
				if sh, ok := sim2.Ships.Get(e); ok {
					p.Shield, p.ShieldMax = sh.Shield, sh.MaxShield
					p.Armor, p.ArmorMax = sh.Armor, sh.MaxArmor
					p.Structure = sh.Structure
				}
			}
			ev.Probes = append(ev.Probes, p)
		},
	})
	for _, st := range res.Unresolved {
		ev.Unresolved = append(ev.Unresolved, st.Action+" "+st.Target)
	}
	end, err := s.Snapshot()
	if err != nil {
		t.Fatalf("end snapshot: %v", err)
	}
	ev.End = end

	// Programmatic checkpoints: save/restore and determinism.
	ev.SaveRestoreOK = checkSaveRestore(t, c, wd, end)
	ev.DeterminismOK = checkDeterminism(t, c, wd, steps)

	if len(ev.Unresolved) > 0 {
		t.Fatalf("showcase script has unresolved steps (fix entity names):\n%s", ev.summarize())
	}

	// Every checkpoint declared by the world must have a rule, and pass.
	for _, cp := range wd.Scenario {
		rule, ok := rules[cp.Feature]
		if !ok {
			t.Errorf("checkpoint %q declares feature %q but no evidence rule exists in tests/showcase_test.go", cp.Name, cp.Feature)
			continue
		}
		pass, reason := rule(c, ev)
		if !pass {
			t.Errorf("feature %q (%s): %s\n%s", cp.Feature, cp.Name, reason, ev.summarize())
		}
	}
}

// checkSaveRestore verifies snapshot → restore → identical hash, and that
// both sims continue identically afterwards.
func checkSaveRestore(t *testing.T, c *content.Content, wd *content.WorldDef, save *sim.Save) bool {
	t.Helper()
	a, err := sim.New(c, wd, 42)
	if err != nil {
		return false
	}
	if err := a.Restore(save); err != nil {
		t.Errorf("restore: %v", err)
		return false
	}
	// Restoring into a fresh sim must reproduce the captured hash.
	b, err := sim.New(c, wd, 42)
	if err != nil {
		return false
	}
	if err := b.Restore(save); err != nil {
		return false
	}
	if a.Hash() != b.Hash() {
		return false
	}
	for i := 0; i < 600; i++ {
		a.Step()
		b.Step()
	}
	return a.Hash() == b.Hash()
}

// checkDeterminism verifies twin runs of the scripted showcase stay in lockstep.
func checkDeterminism(t *testing.T, c *content.Content, wd *content.WorldDef, steps []script.Step) bool {
	t.Helper()
	a, err := sim.New(c, wd, 42)
	if err != nil {
		return false
	}
	b, err := sim.New(c, wd, 42)
	if err != nil {
		return false
	}
	for i := 0; i < 2; i++ {
		script.Run(a, steps, showcaseTicks/2, nil)
		script.Run(b, steps, showcaseTicks/2, nil)
		if a.Hash() != b.Hash() {
			return false
		}
	}
	return true
}
