package sim

import (
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// wreckExpiry is the seconds a wreck stays on the grid before despawning.
const wreckExpiry = 300

// wreckReclaimFraction is the share of a destroyed hull's recipe input bill
// that comes back as reclaimed material on its wreck (floor per input,
// mineral items only).
const wreckReclaimFraction = 0.4

// hullRecipe returns the manufacturing recipe producing the given hull
// (canonical: recipes are scanned in RecipeID order, so the answer is a
// pure function of content). Nil when the hull has no recipe.
func hullRecipe(c *content.Content, hull content.ShipID) *content.RecipeDef {
	for _, r := range c.RecipesInOrder() {
		if r.Product == content.ItemID(hull) {
			return r
		}
	}
	return nil
}

// reclaimedMinerals is the wreck-reprocessing yield of one hull: the floor
// of wreckReclaimFraction of every mineral input of its recipe, e.g. a
// Scuttler (19,900 min-ferrite + 2,800 min-silex) reclaims 7,960 + 1,120.
// Pure math — no RNG, no market movement.
func reclaimedMinerals(c *content.Content, hull content.ShipID) []content.CargoStack {
	rec := hullRecipe(c, hull)
	if rec == nil {
		return nil
	}
	var out []content.CargoStack
	for _, in := range rec.Inputs {
		def, ok := c.Item(in.Item)
		if !ok || def.Category != content.CatMineral {
			continue
		}
		if q := math.Floor(wreckReclaimFraction * in.Qty); q >= 1 {
			out = append(out, content.CargoStack{Item: in.Item, Qty: int(q)})
		}
	}
	return out
}

// systemDamage applies queued damage: shield -> armor -> structure with the
// ship's resists, then resolves deaths.
//
// Resist model (documented): the ship's single resist set is applied once
// at impact; each layer absorbs out of one pooled HP budget and overflow
// carries to the next layer at FULL value (EVE's 50% shield->armor bleed is
// deliberately not used).
func (s *Sim) systemDamage() {
	if len(s.damage) == 0 {
		return
	}
	type death struct{ e, src ecs.Entity }
	var (
		dead     []death
		deadSeen = make(map[ecs.Entity]bool)
	)
	for _, d := range s.damage {
		ship, ok := s.Ships.Get(d.dst)
		if !ok || deadSeen[d.dst] {
			continue
		}
		// Record the damage source on NPCs: aggro trigger (b) answers the
		// attacker itself (systemNpc), and only when it fights for another
		// faction — so a sister patrol's fire, or one rat shooting another,
		// never flips an NPC.
		if ai, ok := s.AIs.Get(d.dst); ok {
			ai.LastHitBy = d.src
		}
		pen := applyDamageToShip(ship, d.dmg)
		text := s.nameOf(d.src) + " hits " + s.nounOf(d.dst) + " for " + num(pen) + " damage"
		if d.wrecking {
			text = "Wrecking hit! " + text
		}
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: d.src, Target: d.dst,
			Text: text, Value: pen})
		if ship.Structure <= 0 {
			ship.Structure = 0
			deadSeen[d.dst] = true
			dead = append(dead, death{e: d.dst, src: d.src})
		}
	}
	s.damage = nil
	// Deaths are resolved after all damage so that simultaneous kills in one
	// tick resolve deterministically in first-damage order.
	for _, d := range dead {
		s.handleDeath(d.e, d.src)
	}
}

// applyDamageToShip reduces the ship's three layers by the penetrating
// damage and returns the total penetrating damage (after resists).
func applyDamageToShip(ship *ShipState, dmg content.Damage) float64 {
	pen := dmg.ApplyTo(ship.Resists).Total()
	pool := pen
	if pool > 0 && ship.Shield > 0 {
		take := math.Min(pool, ship.Shield)
		ship.Shield -= take
		pool -= take
	}
	if pool > 0 && ship.Armor > 0 {
		take := math.Min(pool, ship.Armor)
		ship.Armor -= take
		pool -= take
	}
	if pool > 0 && ship.Structure > 0 {
		take := math.Min(pool, ship.Structure)
		ship.Structure -= take
		pool -= take
	}
	return pen
}

// handleDeath resolves a destroyed ship: NPC deaths roll loot into a wreck,
// pay the bounty to the player (player kills only), record respawn timers
// and possibly clear an anomaly; the player respawns in a fresh starter
// hull at a station, losing cargo but keeping wallet and hangar (EVE
// spirit: pod-less respawn, documented).
func (s *Sim) handleDeath(e, src ecs.Entity) {
	if s.Pilots.Has(e) {
		hullName := s.nameOf(e)
		s.emit(Event{Kind: EvDestroyed, Severity: SevDanger, Actor: e,
			Text: "Your " + hullName + " was destroyed"})
		// Respawn is deferred to the end of the Step: a clone-bay respawn may
		// rebuild the whole grid (another system), which must not happen
		// while the remaining deaths and systems are still resolving.
		s.pendingPlayerRespawn = true
		return
	}

	ai, hasAI := s.AIs.Get(e)
	name := s.nameOf(e)
	pos, _ := s.posOf(e)

	// Player-attributed NPC kills are counted toward kill contracts by
	// systemMission later in this Step.
	if hasAI && src == s.player {
		s.pendingKills = append(s.pendingKills, ai.Npc)
	}

	var def *content.NpcDef
	if hasAI {
		def, _ = s.Content.Npc(ai.Npc)
	}

	// Loot roll from the dying NPC's own RNG stream.
	var loot []content.CargoStack
	if def != nil && hasAI {
		rng := ai.Rand
		if rng == nil {
			rng = s.RNG
		}
		for _, drop := range def.Loot {
			if !rng.Chance(drop.Chance) {
				continue
			}
			qty := drop.MinQty
			if drop.MaxQty > drop.MinQty {
				qty += rng.Intn(drop.MaxQty - drop.MinQty + 1)
			}
			if qty > 0 {
				if _, ok := s.Content.Item(drop.Item); ok {
					loot = addStacks(loot, drop.Item, qty)
				}
			}
		}
	}

	// Convoy haulers carry their shipment instead of a rat loot table: the
	// wreck gets the cargo (lootable through the normal CmdLoot path), and
	// the destination market takes the lost-supply risk premium.
	var tc *Traffic
	if t, ok := s.Traffic.Get(e); ok {
		tc = t
		if tc.Qty > 0 && tc.Item != "" {
			loot = addStacks(loot, tc.Item, tc.Qty)
		}
		s.trafficHaulerDestroyed(e, tc)
	}

	// A miner's ore hold rides its wreck: the load it cut is lootable back
	// (the ordinary wreck path carries the hold, not the loot table).
	if def != nil && def.IsMiner() {
		if c, ok := s.Cargos.Get(e); ok {
			for _, st := range c.Ore {
				loot = addStacks(loot, st.Item, st.Qty)
			}
		}
	}

	// Wreck reprocessing: a destroyed ship whose hull has a manufacturing
	// recipe returns part of its mineral bill as reclaimed material, so
	// wrecks are organic material sources without conjuring products.
	// (Fitted modules are not reprocessed in v1 — documented simplification.)
	if sh, ok := s.Ships.Get(e); ok {
		for _, st := range reclaimedMinerals(s.Content, sh.Hull) {
			loot = addStacks(loot, st.Item, st.Qty)
		}
	}

	// Bounty: only kills attributed to the player pay out.
	if def != nil && def.Bounty > 0 && src == s.player {
		s.wallet += def.Bounty
		s.recordLedger(LedgerEntry{
			What:   "Bounty — " + name,
			Where:  s.systemName(),
			Amount: def.Bounty,
		})
		s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player, Target: e,
			Text: "Bounty paid: " + euro(def.Bounty), Value: def.Bounty})
	}

	// The dying ship's bays are part of the wreck: rounds loaded when it came
	// apart are lootable with the rest (cargo stays where it always did — the
	// wreck carries the bay only).
	if fit, ok := s.Fittings.Get(e); ok {
		for _, class := range slotClasses {
			rts := fit.Runtimes(class)
			for i := range rts {
				if rt := &rts[i]; rt.Clip.Qty > 0 {
					loot = addStacks(loot, rt.Clip.Item, rt.Clip.Qty)
					rt.Clip = content.CargoStack{}
				}
			}
		}
	}

	if len(loot) > 0 {
		w := s.create()
		s.Names.Set(w, Name{Value: name + " Wreck"})
		s.Transforms.Set(w, Transform{Pos: pos})
		s.Wrecks.Set(w, Wreck{Loot: loot, ExpiresAt: s.time + wreckExpiry})
	}

	s.emit(Event{Kind: EvDestroyed, Severity: SevInfo, Actor: e, Target: src,
		Text: name + " destroyed"})

	if hasAI {
		s.recordRespawn(e, ai)
		s.checkAnomalyCleared(ai)
		s.checkBeltCleared(ai)
	}
	s.destroy(e)
}

// recordRespawn queues a respawn timer for a dead (or fled) NPC when its
// spawn group has RespawnSec > 0. The owning celestial and the respawn
// cadence are re-derived from Home + the system definition so no extra
// state needs persisting across save/load. Traffic ambushers are skipped:
// they are not members of any spawn group, and their deaths near a belt or
// anomaly must not queue timers against that group.
func (s *Sim) recordRespawn(e ecs.Entity, ai *NpcAI) {
	if s.isTrafficRaider(e) {
		return
	}
	cel := s.nearestCelestial(ai.Home)
	if cel.IsZero() {
		return
	}
	c, ok := s.Celestials.Get(cel)
	if !ok {
		return
	}
	sys := s.System()
	if sys == nil {
		return
	}
	var celDef *content.CelestialDef
	for i := range sys.Celestials {
		if sys.Celestials[i].ID == c.Cel {
			celDef = &sys.Celestials[i]
			break
		}
	}
	if celDef == nil {
		return
	}
	sec := respawnSecOf(celDef, ai.Npc)
	if sec <= 0 {
		return
	}
	s.respawns = append(s.respawns, RespawnTimer{
		System: s.systemID, Cel: celDef.ID, Npc: ai.Npc, At: s.time + sec,
	})
}

// respawnSecOf finds the respawn cadence of one NPC at one celestial.
func respawnSecOf(cel *content.CelestialDef, npc content.NpcID) float64 {
	for _, sp := range cel.Spawns {
		if sp.NpcID == npc {
			return sp.RespawnSec
		}
	}
	if cel.Anomaly != nil {
		for _, sp := range cel.Anomaly.Spawns {
			if sp.NpcID == npc {
				return cel.Anomaly.RespawnSec
			}
		}
	}
	return 0
}

// checkAnomalyCleared emits EvtAnomalyCleared when the last NPC spawned at
// an anomaly celestial dies (the dying NPC itself is skipped by identity).
func (s *Sim) checkAnomalyCleared(ai *NpcAI) {
	cel := s.nearestCelestial(ai.Home)
	if cel.IsZero() {
		return
	}
	c, ok := s.Celestials.Get(cel)
	if !ok || c.Kind != content.CelAnomaly {
		return
	}
	for _, e := range sortedEntities(s.AIs) {
		other, ok := s.AIs.Get(e)
		if !ok || other == ai {
			continue // the dying NPC is still in the store at this point
		}
		if other.Home == ai.Home {
			return // a member survives: the anomaly is not cleared
		}
	}
	s.emit(Event{Kind: EvAnomalyCleared, Severity: SevGood, Actor: s.player,
		Text: s.nameOf(cel) + " cleared"})
}

// checkBeltCleared reports the good news when the last live rat of a
// HIGH-sec belt dies — whoever landed the kill (the dying rat is skipped by
// identity). The belt's own respawn timers re-arm the rats, so this is a
// moment of news, not a permanent state. Rats only: a guard dying at a belt
// is not a cleared belt.
func (s *Sim) checkBeltCleared(ai *NpcAI) {
	sys := s.System()
	if sys == nil || sys.Security != "high" {
		return
	}
	if def, ok := s.Content.Npc(ai.Npc); !ok || !isRatNpc(def) {
		return
	}
	cel := s.nearestCelestial(ai.Home)
	if cel.IsZero() {
		return
	}
	c, ok := s.Celestials.Get(cel)
	if !ok || c.Kind != content.CelBelt {
		return
	}
	// The dying rat is still in the store at this point (destroy runs after
	// this hook), so the survivor scan skips it by identity.
	for _, e := range sortedEntities(s.AIs) {
		other, ok := s.AIs.Get(e)
		if !ok || other == ai || other.Home != ai.Home {
			continue
		}
		if def, ok := s.Content.Npc(other.Npc); ok && isRatNpc(def) {
			return // a rat survives: the belt is not cleared
		}
	}
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: cel,
		Text: s.nameOf(cel) + " cleared"})
}

// respawnPlayer rebuilds the player in a fresh starter hull at the respawn
// station (last docked station, else the system's first station).
func (s *Sim) respawnPlayer() {
	p := s.player

	// Choose the respawn station.
	stationE := s.stationAt(s.lastStation)
	if stationE.IsZero() {
		if sys := s.System(); sys != nil {
			stationE = s.stationAt(firstStationID(sys))
		}
	}
	var stPos core.Vec2
	stName := ""
	if !stationE.IsZero() {
		stPos, _ = s.posOf(stationE)
		stName = s.nameOf(stationE)
	}

	// Clone bay: when this system holds no station to wake up in, respawn at
	// the home station in the universe's start system (EVE clone semantics).
	// Safe to rebuild the grid here — respawnPlayer runs at the end of a Step.
	if stationE.IsZero() {
		if u := s.Content.Universe; u != nil {
			if home, ok := s.Content.System(u.StartSystem); ok {
				if cel := firstStationID(home); cel != "" {
					_ = s.LoadSystem(u.StartSystem, cel)
					stationE = s.stationAt(cel)
					if !stationE.IsZero() {
						stPos, _ = s.posOf(stationE)
						stName = s.nameOf(stationE)
					}
				}
			}
		}
	}

	// Choose the hull: the canonical starter sloop, else the world start ship.
	hullID := content.ShipID("ship-sloop")
	hull, ok := s.Content.Ship(hullID)
	if !ok {
		if w, ok := s.Content.World(s.worldID); ok {
			hull, ok = s.Content.Ship(w.Start.Ship)
			if ok {
				hullID = w.Start.Ship
			}
		}
		if !ok {
			return // no hull available: keep the wrecked state (content error)
		}
	}

	fitIDs := []content.ModuleID{}
	if _, ok := s.Content.Module("mod-autocannon-s"); ok {
		fitIDs = append(fitIDs, "mod-autocannon-s")
	}
	fit, ferr := s.buildFitting(fitIDs, hull)
	if ferr != nil {
		fit = Fitting{}
	}

	if tr, ok := s.Transforms.Get(p); ok && !stationE.IsZero() {
		tr.Pos = stPos.Add(core.Vec2{X: 5000})
	}
	if m, ok := s.Motions.Get(p); ok {
		*m = Motion{
			Mass: hull.Mass, MaxSpeed: hull.MaxSpeed, Agility: hull.Agility,
			SigRadius: hull.SigRadius, SpeedMultiplier: 1, WebFactor: 1,
		}
	}
	s.Ships.Set(p, ShipState{
		Hull: hull.ID, Shield: hull.Shield, Armor: hull.Armor, Structure: hull.Structure,
		MaxShield: hull.Shield, MaxArmor: hull.Armor, MaxStructure: hull.Structure,
		Resists: hull.Resists,
	})
	s.Caps.Set(p, Capacitor{Current: hull.Capacitor, Capacity: hull.Capacitor, Tau: hull.Recharge})
	s.Cargos.Set(p, Cargo{Capacity: hull.Cargo, OreCapacity: hull.OreHold}) // cargo lost
	s.Fittings.Set(p, fit)
	s.Targetings.Set(p, Targeting{
		MaxTargets: hull.MaxTargets, MaxRange: hull.MaxTargetRange, ScanRes: hull.ScanResolution,
	})
	s.Intents.Remove(p)
	s.Warps.Remove(p)
	s.Docked.Remove(p)
	s.recomputeFit(p)

	if stName != "" {
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: p,
			Text: "Respawned at " + stName + " in a " + hull.Name})
	} else {
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: p,
			Text: "Respawned in a " + hull.Name})
	}
}
