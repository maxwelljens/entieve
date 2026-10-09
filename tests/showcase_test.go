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
	"fmt"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

const (
	showcaseWorldID = "world-showcase"
	showcaseScript  = "scripts/showcase.json"
	showcaseTicks   = 60_000 // 16.6 sim minutes; covers the full script tour
	probeEveryTicks = 300    // 5 sim seconds (state vocabularies track every tick)
)

// featureRule observes one feature in the evidence gathered from the run.
type featureRule func(c *content.Content, e *evidence) (ok bool, reason string)

// rules is the evidence registry. Keys are the `feature` strings used by
// world-showcase's scenario checkpoints (kebab-case).
var rules = map[string]featureRule{
	"fitting-all-kinds": func(c *content.Content, e *evidence) (bool, string) {
		// The world-start fit is the showcase rack: death respawns hand out a
		// bare sloop, so judge the declared loadout, not the end state.
		kinds := map[string]bool{}
		p := playerOf(e.Start)
		if p == nil || p.Fitting == nil {
			return false, "no player fitting at world start"
		}
		ids := append(append([]content.ModuleID{}, p.Fitting.High...), p.Fitting.Mid...)
		ids = append(ids, p.Fitting.Low...)
		for _, id := range ids {
			if def, ok := c.Module(id); ok {
				kinds[def.Kind] = true
			}
		}
		for _, want := range []string{content.KindWeapon, content.KindMiningLaser, content.KindAfterburner} {
			if !kinds[want] {
				return false, "start fit has no module of kind " + want
			}
		}
		if !kinds[content.KindShieldBoost] && !kinds[content.KindArmorRep] {
			return false, "start fit has no active tank module"
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
		return e.eventRe(`no charges|dry|out of ammo`), "no dry-gun warning"
	},
	"turret-hit": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`\bhits?\b|damage`), "no turret hit observed"
	},
	"wrecking-shot": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`wrecking`), "no wrecking shot observed"
	},
	"four-damage-types": func(c *content.Content, e *evidence) (bool, string) {
		if e.allChargeTypesBurned() {
			return true, ""
		}
		return false, "not all four charge damage profiles were consumed"
	},
	"resists": func(c *content.Content, e *evidence) (bool, string) {
		hits := e.playerHits()
		if len(hits) == 0 {
			return false, "no player hits to compare against resist ceilings"
		}
		checked := 0
		byTarget := map[string][]float64{}
		for _, h := range hits {
			byTarget[h.Target] = append(byTarget[h.Target], h.Value)
		}
		for name, vals := range byTarget {
			hull, fit := hullForTarget(c, name)
			if hull == nil {
				continue // not a rat hull we can price
			}
			ceiling := resistCeiling(c, hull, fit)
			max := 0.0
			for _, v := range vals {
				if v > max {
					max = v
				}
				if v > ceiling*1.01 {
					return false, fmt.Sprintf("%s took %.2f damage in one hit, above the resist ceiling %.2f", name, v, ceiling)
				}
			}
			// The "multipliers approach 1" check only makes sense with enough
			// rolls: expect max mult ~n/(n+1) over n hits.
			if len(vals) >= 8 && max < ceiling*0.55 {
				return false, fmt.Sprintf("%s hits never approached the resist ceiling %.2f (max %.2f over %d hits)", name, ceiling, max, len(vals))
			}
			checked++
		}
		if checked == 0 {
			return false, "no rat-hull hits observed"
		}
		return true, ""
	},
	"capacitor-drain": func(c *content.Content, e *evidence) (bool, string) {
		return e.minCapFrac() < 0.5, "capacitor never drained below 50%"
	},
	"capacitor-recharge": func(c *content.Content, e *evidence) (bool, string) {
		return e.capDrainThenRecover(0.5, 0.8), "capacitor never drained then recovered"
	},
	"cap-booster": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`cap(acitor)? ?(booster|cell|restor)`), "no cap booster activation"
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
		return e.IntentsSeen[string(sim.IntentApproach)] || e.eventRe(`approach`) || e.movedSublight(50, 2e6),
			"no sublight approach observed"
	},
	"orbit": func(c *content.Content, e *evidence) (bool, string) {
		return e.IntentsSeen[string(sim.IntentOrbit)] || e.eventRe(`orbit`), "no orbit observed"
	},
	"manual-piloting": func(c *content.Content, e *evidence) (bool, string) {
		return e.IntentsSeen[string(sim.IntentManual)] || e.eventRe(`manual`), "no manual piloting observed"
	},
	"npc-aggro": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`attacking you|aggro|hostile|engaging`) ||
				e.AIStatesSeen[string(sim.NpcApproach)] || e.AIStatesSeen[string(sim.NpcOrbit)],
			"no NPC aggro observed"
	},
	"npc-flee": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`retreat|flee`) || e.AIStatesSeen[string(sim.NpcFlee)], "no NPC flee observed"
	},
	"npc-death": func(c *content.Content, e *evidence) (bool, string) {
		// A death that is NOT the player's (the player can also die here).
		p := playerOf(e.Start)
		var playerID uint64
		if p != nil {
			playerID = p.ID
		}
		for _, ev := range e.Events {
			if ev.Kind == sim.EvDestroyed && uint64(ev.Actor) != playerID {
				return true, ""
			}
		}
		return false, "no NPC death observed"
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
		return e.eventRe(`transfer|hangar|stored|delivered`), "no cargo transfer observed"
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
		return e.eventRe(`repair|full strength`), "no station repair observed"
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
		return e.eventRe(`respawn|spawned|sensor contact`), "no NPC respawn observed"
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
	ev := &evidence{
		Start:        start,
		IntentsSeen:  map[string]bool{},
		AIStatesSeen: map[string]bool{},
	}
	res := script.RunH(s, steps, showcaseTicks, script.Hooks{
		OnEvent: func(e sim.Event) { ev.Events = append(ev.Events, e) },
		OnTick: func(tick int64, eng script.Engine) {
			sim2, ok := eng.(*sim.Sim)
			if !ok {
				return
			}
			p := sim2.Player()
			// State vocabularies accumulate on every tick — no sampling gaps.
			if !p.IsZero() {
				if in, ok := sim2.Intents.Get(p); ok {
					ev.IntentsSeen[string(in.Kind)] = true
				}
			}
			sim2.AIs.Each(func(_ ecs.Entity, ai *sim.NpcAI) {
				ev.AIStatesSeen[string(ai.State)] = true
			})
			// Probes sample the player's vitals for curve-based rules.
			if tick%probeEveryTicks != 0 {
				return
			}
			pr := probe{Tick: tick}
			if !p.IsZero() {
				if tr, ok := sim2.Transforms.Get(p); ok {
					pr.Pos = tr.Pos
				}
				if mo, ok := sim2.Motions.Get(p); ok {
					pr.Speed = mo.Vel.Len()
				}
				if cp, ok := sim2.Caps.Get(p); ok {
					pr.Cap, pr.CapMax = cp.Current, cp.Capacity
				}
				if sh, ok := sim2.Ships.Get(p); ok {
					pr.Shield, pr.ShieldMax = sh.Shield, sh.MaxShield
					pr.Armor, pr.ArmorMax = sh.Armor, sh.MaxArmor
					pr.Structure = sh.Structure
				}
			}
			ev.Probes = append(ev.Probes, pr)
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
// restored sims continue identically afterwards.
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
