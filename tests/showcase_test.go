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
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

const (
	showcaseWorldID = "world-showcase"
	showcaseScript  = "scripts/showcase.json"
	// 33.6 sim-minutes: the Młondan belt fight, the 600 s respawn watched
	// docked (only the loaded system's timers tick), the two Solace belt
	// passes it takes to spend a rock, and the war's last beat. The two
	// waits are forced; see scripts/showcase.json's notes.
	showcaseTicks   = 121_000
	probeEveryTicks = 300 // 5 sim seconds (state vocabularies track every tick)
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
		for _, id := range []content.ItemID{"ammo-emp-s", "ammo-fusion-s", "ammo-ap-s", "ammo-plutonium-s"} {
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
			ceiling := resistCeiling(c, hull, fit, playerOf(e.Start))
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
		// The success verb only — "…to the station hangar" errors must not match.
		return e.eventRe(`transferred`), "no cargo transfer observed"
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
	"npc-patrol": func(c *content.Content, e *evidence) (bool, string) {
		return e.IdlePatrolMoved, "no idle NPC was observed patrolling near its anchor"
	},
	"sensor-visibility": func(c *content.Content, e *evidence) (bool, string) {
		if e.SawVisibleAsteroids && e.SawHiddenAsteroids {
			return true, ""
		}
		return false, "asteroids never flipped visible/hidden with sensor range"
	},
	"ammo-selection": func(c *content.Content, e *evidence) (bool, string) {
		return e.eventRe(`loaded with|auto ammo`), "no ammo selection event"
	},
	"warp-to-range": func(c *content.Content, e *evidence) (bool, string) {
		return e.SawRangedWarpLanding, "player never landed at a ranged warp distance from a station"
	},
	"contract-loop": func(c *content.Content, e *evidence) (bool, string) {
		if !e.kindSeen(sim.EvMission, sim.MissionAccepted) {
			return false, "no contract accepted at the station board"
		}
		if !e.kindSeen(sim.EvMission, sim.MissionProgress) {
			return false, "accepted contract never counted objective progress"
		}
		if !e.kindSeen(sim.EvMission, sim.MissionCompleted) {
			return false, "no contract auto-claimed on docking at the offering station"
		}
		paid := false
		for _, ev := range e.Events {
			if ev.Kind == sim.EvMission && ev.Phase == sim.MissionCompleted && ev.Value > 0 {
				paid = true
			}
		}
		if !paid {
			return false, "completed contract never transferred credits"
		}
		if e.Start == nil || e.End == nil || e.End.Standing <= e.Start.Standing {
			return false, "contract completion never credited standing"
		}
		return true, ""
	},
	"save-restore": func(c *content.Content, e *evidence) (bool, string) {
		return e.SaveRestoreOK, "save/restore round-trip failed"
	},
	"determinism": func(c *content.Content, e *evidence) (bool, string) {
		return e.DeterminismOK, "twin runs diverged"
	},
	"traffic-delivery": func(c *content.Content, e *evidence) (bool, string) {
		if !e.kindSeen(sim.EvTraffic, sim.TrafficDelivered) {
			return false, "no convoy delivery event"
		}
		if !e.eventRe(`price falls to`) {
			return false, "no market price drift observed"
		}
		// The run must end with at least one good priced below its base —
		// supply from deliveries actually moved the market, not just logs.
		basePrice := func(sys content.SystemID, item content.ItemID) float64 {
			if def, ok := c.System(sys); ok {
				for _, o := range def.Market {
					if o.Item == item {
						return o.Price
					}
				}
			}
			if def, ok := c.Item(item); ok {
				return def.BasePrice
			}
			return 0
		}
		if e.End != nil {
			for sys, stock := range e.End.Market {
				for _, st := range stock {
					if b := basePrice(sys, st.Item); b > 0 && st.Price < b {
						return true, ""
					}
				}
			}
		}
		return false, "no market price ended below its base"
	},
	"traffic-attack": func(c *content.Content, e *evidence) (bool, string) {
		for _, ev := range e.Events {
			if ev.Kind == sim.EvTraffic && ev.Phase == sim.TrafficAttacked &&
				ev.System == "sys-penumbra" {
				return true, ""
			}
		}
		return false, "no ambush rolled in the Penumbra legs"
	},
	"traffic-patrol": func(c *content.Content, e *evidence) (bool, string) {
		response, escaped := false, false
		for _, ev := range e.Events {
			if ev.Kind != sim.EvTraffic || ev.System != "sys-penumbra" {
				continue
			}
			switch ev.Phase {
			case sim.TrafficPatrolResponse:
				response = true
			case sim.TrafficEscaped:
				escaped = true
			}
		}
		if !response {
			return false, "no patrol response dispatched in Penumbra"
		}
		if !escaped {
			return false, "patrol responded but the convoy never warped clear"
		}
		return true, ""
	},

	// --- the Long Attrition -------------------------------------------------
	"war-procurement": func(c *content.Content, e *evidence) (bool, string) {
		// A war bid only fills via settle (EvWar supply) and only for the
		// materiel the war buys; the goods leave the hold either way, so the
		// pair proves the sink: paid, consumed, credited.
		for _, ev := range e.Events {
			if ev.Kind != sim.EvWar || ev.Phase != sim.WarSupply || ev.Value <= 0 || ev.Item == "" {
				continue
			}
			if e.itemQtyDropped(ev.Item) > 0 {
				return true, ""
			}
		}
		return false, "no war-bid sale of military goods was observed"
	},
	"war-contest": func(c *content.Content, e *evidence) (bool, string) {
		for _, ev := range e.Events {
			if ev.Kind == sim.EvWar && ev.Phase == sim.WarContest && ev.System != "" &&
				ev.Value > 0 && ev.Value < 1 {
				return true, ""
			}
		}
		return false, "no contest line on the front"
	},
	"war-fall": func(c *content.Content, e *evidence) (bool, string) {
		for _, ev := range e.Events {
			if ev.Kind == sim.EvWar && ev.Phase == sim.WarFlip && ev.Severity == sim.SevDanger &&
				strings.Contains(ev.Text, "lost to the Old Works") {
				return true, ""
			}
		}
		return false, "no system fell to the Old Works"
	},
	"war-reclaim": func(c *content.Content, e *evidence) (bool, string) {
		for _, ev := range e.Events {
			if ev.Kind == sim.EvWar && ev.Phase == sim.WarFlip && ev.Severity == sim.SevGood &&
				strings.Contains(ev.Text, "returns to the charter") {
				return true, ""
			}
		}
		return false, "no system came back to the charter"
	},
	"war-remnant": func(c *content.Content, e *evidence) (bool, string) {
		if checkWarRemnant(c) {
			return true, ""
		}
		return false, "a docked pilot in fallen space was served like charter space"
	},
	"war-fleet": func(c *content.Content, e *evidence) (bool, string) {
		for _, ev := range e.Events {
			if ev.Kind == sim.EvWar && ev.Phase == sim.WarSkirmish {
				return true, ""
			}
		}
		return false, "no Corporate Fleets skirmish at the front"
	},
	"war-news": func(c *content.Content, e *evidence) (bool, string) {
		seen := map[string]bool{}
		for _, ev := range e.Events {
			if ev.Kind != sim.EvWar {
				continue
			}
			seen[ev.Phase] = true
			if !strings.Contains(ev.Text, " — ") {
				return false, "war line is not the 'Phase — detail' shape: " + ev.Text
			}
		}
		for _, phase := range []string{sim.WarSupply, sim.WarContest, sim.WarFlip, sim.WarSkirmish} {
			if !seen[phase] {
				return false, "no " + phase + " line in the log"
			}
		}
		return true, ""
	},
}

// checkWarRemnant proves the remnant behavior the tour cannot reach: the tour's
// ship is warp-scrambled in its last fight, so it cannot dock at Hauvnok after
// the fall. This replays the same run to the Kvonice gate, flies to the front
// station instead, drives the fall with the script harness's war lever and
// checks the station: the charter desk refuses (services dark) while the
// berth, market and repair keep working.
func checkWarRemnant(c *content.Content) bool {
	wd, ok := c.World(showcaseWorldID)
	if !ok {
		return false
	}
	data, err := readFile(showcaseScript)
	if err != nil {
		return false
	}
	all, err := script.Parse(data)
	if err != nil {
		return false
	}
	// The last Gate (Kvonice) jump is the tour's arrival in the Fringe; the
	// probe re-anchors on it so a re-timed tour does not need a new literal.
	var cut int64
	for _, st := range all {
		if st.Action == "jump" && st.Target == "Gate (Kvonice)" {
			cut = st.AtTick
		}
	}
	if cut == 0 {
		return false
	}
	// warBeatTicks mirrors sim's procureCadence: pushes land on the hourly beat.
	const warBeatTicks = 3_600
	beat := (cut + 1_200 + warBeatTicks - 1) / warBeatTicks * warBeatTicks
	var steps []script.Step
	for _, st := range all {
		if st.AtTick <= cut { // ...through the jump into Kvonice
			steps = append(steps, st)
		}
	}
	steps = append(steps,
		script.Step{AtTick: cut + 60, Action: "warp", Target: "Hauvnok"},
		script.Step{AtTick: cut + 940, Action: "dock", Target: "Hauvnok"},
		// The front applies a push on its hourly beat (sim/system_war.go
		// procureCadence), so the lever lands just before the next beat and
		// the desk is probed right after it.
		script.Step{AtTick: beat - 60, Action: "war-push", System: "sys-penumbra", Value: 1.2},
		script.Step{AtTick: beat + 120, Action: "accept-mission", Mission: "contract-penumbra-den-hunt-1"},
		script.Step{AtTick: beat + 180, Action: "sell", Item: "ore-cordite", Qty: 10},
	)
	s, err := sim.New(c, wd, 42)
	if err != nil {
		return false
	}
	fell, refused, sold := false, false, false
	script.RunH(s, steps, beat+600, script.Hooks{OnEvent: func(ev sim.Event) {
		switch {
		case ev.Kind == sim.EvWar && ev.Phase == sim.WarFlip && ev.System == "sys-penumbra":
			fell = true
		case ev.Kind == sim.EvMission && ev.Phase == sim.MissionRefused &&
			strings.Contains(ev.Text, "remnant"):
			refused = fell
		case ev.Kind == sim.EvMarket && strings.Contains(ev.Text, "Sold"):
			sold = sold || fell
		}
	}})
	if !fell || !refused || !sold {
		return false
	}
	w := s.War()
	return w != nil && w.Control["sys-penumbra"] == "works"
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
		npcFirstPos:  map[uint64]core.Vec2{},
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
			sim2.AIs.Each(func(e ecs.Entity, ai *sim.NpcAI) {
				ev.AIStatesSeen[string(ai.State)] = true
				// Patrol evidence: idle NPCs must drift around their anchor.
				if ai.State != sim.NpcIdle {
					return
				}
				tr, ok := sim2.Transforms.Get(e)
				if !ok {
					return
				}
				fp, seen := ev.npcFirstPos[uint64(e)]
				if !seen {
					ev.npcFirstPos[uint64(e)] = tr.Pos
					return
				}
				if tr.Pos.Dist(fp) > 50 {
					ev.IdlePatrolMoved = true
				}
			})
			// Sensor visibility: rocks in/out of SensorRange (landmarks like
			// belt markers are excluded — they are always visible by design).
			visibleRocks := 0
			sim2.Asteroids.Each(func(e ecs.Entity, _ *sim.Asteroid) {
				if in, ok := sim2.Info(e); ok && in.Visible && in.Kind == sim.KindAsteroid {
					visibleRocks++
				}
			})
			if visibleRocks > 0 {
				ev.SawVisibleAsteroids = true
			} else {
				ev.SawHiddenAsteroids = true
			}
			// Ranged-warp band: close enough to a station to see it on grid,
			// far enough that a 50 km warp-to-within clearly landed short.
			sim2.Stations.Each(func(e ecs.Entity, _ *sim.Station) {
				if in, ok := sim2.Info(e); ok && in.Distance > 15000 && in.Distance < 120000 {
					ev.SawRangedWarpLanding = true
				}
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
