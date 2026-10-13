package sim

import (
	"math"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// initMarket seeds every station's founding book from the content
// definitions, in deterministic order (sorted system ID, celestial order
// within a system). Founding prices are quantized to the price tick so the
// never-crossed invariant starts true for every book. The second result is
// the next free order id (one arrival id per seeded order).
func initMarket(c *content.Content) (map[StationID]*MarketBook, uint64) {
	m := make(map[StationID]*MarketBook, len(c.Systems))
	ids := make([]content.SystemID, 0, len(c.Systems))
	for id := range c.Systems {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	seq := uint64(0)
	for _, id := range ids {
		sys := c.Systems[id]
		for _, cel := range sys.Celestials {
			if cel.Station == nil || len(cel.Station.Market) == 0 {
				continue
			}
			book := &MarketBook{System: id, Lasts: map[content.ItemID]float64{}}
			for _, o := range cel.Station.Market {
				seq++
				side := OrderSell
				if o.Side == "buy" {
					side = OrderBuy
				}
				book.Orders = append(book.Orders, MarketStock{
					Item: o.Item, Price: quantizeTick(o.Price), Qty: o.Qty,
					Side: side, Owner: OwnerSystem, Station: cel.ID, ID: seq,
				})
			}
			book.sortOrders()
			m[cel.ID] = book
		}
	}
	return m, seq
}

// buildFitting partitions a flat module list (high slots first, then mid,
// then low — the WorldDef.Start.Fit / StoredShip.Fit convention) into slot
// classes, validating existence, slot class and hull capacity.
func (s *Sim) buildFitting(ids []content.ModuleID, hull *content.ShipDef) (Fitting, error) {
	var f Fitting
	for _, id := range ids {
		def, ok := s.Content.Module(id)
		if !ok {
			return f, errf("sim: unknown module %s in fit", id)
		}
		rt := ModuleRuntime{Def: id}
		switch def.Slot {
		case content.SlotHigh:
			if len(f.High) >= hull.Slots.High {
				return f, errf("sim: high slot overflow fitting %s", id)
			}
			f.High = append(f.High, id)
			f.HighRT = append(f.HighRT, rt)
		case content.SlotMid:
			if len(f.Mid) >= hull.Slots.Mid {
				return f, errf("sim: mid slot overflow fitting %s", id)
			}
			f.Mid = append(f.Mid, id)
			f.MidRT = append(f.MidRT, rt)
		case content.SlotLow:
			if len(f.Low) >= hull.Slots.Low {
				return f, errf("sim: low slot overflow fitting %s", id)
			}
			f.Low = append(f.Low, id)
			f.LowRT = append(f.LowRT, rt)
		default:
			return f, errf("sim: module %s has invalid slot %q", id, def.Slot)
		}
	}
	return f, nil
}

// flatFit flattens a fitting back into the high-then-mid-then-low order.
func flatFit(f *Fitting) []content.ModuleID {
	out := make([]content.ModuleID, 0, len(f.High)+len(f.Mid)+len(f.Low))
	out = append(out, f.High...)
	out = append(out, f.Mid...)
	out = append(out, f.Low...)
	return out
}

// buildPlayer creates the player entity from the world start definition.
func (s *Sim) buildPlayer(w *content.WorldDef) error {
	hull, _ := s.Content.Ship(w.Start.Ship)

	e := s.create()
	s.player = e
	s.Names.Set(e, Name{Value: hull.Name})
	s.Transforms.Set(e, Transform{Pos: w.Start.Position})
	s.Motions.Set(e, Motion{
		Mass: hull.Mass, MaxSpeed: hull.MaxSpeed, Agility: hull.Agility,
		SigRadius: hull.SigRadius, SpeedMultiplier: 1, WebFactor: 1,
	})
	s.Ships.Set(e, ShipState{
		Hull: hull.ID, Shield: hull.Shield, Armor: hull.Armor, Structure: hull.Structure,
		MaxShield: hull.Shield, MaxArmor: hull.Armor, MaxStructure: hull.Structure,
		Resists: hull.Resists,
	})
	s.Caps.Set(e, Capacitor{Current: hull.Capacitor, Capacity: hull.Capacitor, Tau: hull.Recharge})

	cargo := Cargo{Capacity: hull.Cargo, OreCapacity: hull.OreHold}
	for _, st := range w.Start.Cargo {
		if st.Qty <= 0 {
			continue
		}
		if _, ok := s.Content.Item(st.Item); !ok {
			return errf("sim: world %s starts with unknown cargo item %s", w.ID, st.Item)
		}
		cargo.Stacks = addStacks(cargo.Stacks, st.Item, st.Qty)
	}
	s.Cargos.Set(e, cargo)

	fit, err := s.buildFitting(w.Start.Fit, hull)
	if err != nil {
		return err
	}
	s.Fittings.Set(e, fit)
	s.Targetings.Set(e, Targeting{
		MaxTargets: hull.MaxTargets, MaxRange: hull.MaxTargetRange, ScanRes: hull.ScanResolution,
	})
	s.Pilots.Set(e, Pilot{Player: true})
	s.recomputeFit(e)
	s.healToFull(e) // passive modules raise the maxima; spawn at full strength
	return nil
}

// loadSystem rebuilds the grid for the given system. All entities except the
// player are destroyed; the player's ship, cargo, wallet and hangar survive.
// An empty entryAtCel keeps the player's current position.
func (s *Sim) loadSystem(id content.SystemID, entryAtCel content.CelID) error {
	sys, ok := s.Content.System(id)
	if !ok {
		return errf("sim: unknown system %s", id)
	}
	for _, e := range s.allEntities() {
		if e != s.player {
			s.destroy(e)
		}
	}
	s.systemID = id

	// Reset grid-local player state (warp, motion, locks, movement intent).
	if !s.player.IsZero() {
		if m, ok := s.Motions.Get(s.player); ok {
			m.Vel = core.Vec2{}
			m.WebbedUntil = 0
		}
		s.Warps.Remove(s.player)
		s.Intents.Remove(s.player)
		if t, ok := s.Targetings.Get(s.player); ok {
			t.Locks = nil
		}
		if sh, ok := s.Ships.Get(s.player); ok {
			sh.ScrambledUntil = 0
		}
	}
	// Positions are per-system: work recorded elsewhere must not aggro this
	// system's guards.
	s.lastPlayerWork.pos, s.lastPlayerWork.at = core.Vec2{}, playerWorkNever

	// Pending respawn timers of THIS system reduce the fresh spawn counts so
	// that a group never exceeds its declared Count across jumps.
	pending := make(map[string]int)
	for _, rt := range s.respawns {
		if rt.System == id {
			pending[string(rt.Cel)+"|"+string(rt.Npc)]++
		}
	}

	for i := range sys.Celestials {
		s.spawnCelestial(&sys.Celestials[i], pending)
	}

	if entryAtCel != "" && !s.player.IsZero() {
		var found bool
		for i := range sys.Celestials {
			if sys.Celestials[i].ID == entryAtCel {
				s.Transforms.Set(s.player, Transform{Pos: sys.Celestials[i].Position})
				found = true
				break
			}
		}
		if !found {
			return errf("sim: system %s has no celestial %s", id, entryAtCel)
		}
	}
	return nil
}

// spawnCelestial creates the grid entity (and its children: belt rocks and
// NPC spawns) for one celestial definition.
func (s *Sim) spawnCelestial(cel *content.CelestialDef, pending map[string]int) {
	e := s.create()
	s.Names.Set(e, Name{Value: cel.Name})
	s.Transforms.Set(e, Transform{Pos: cel.Position})
	s.Celestials.Set(e, Celestial{Kind: cel.Kind, Cel: cel.ID, Radius: cel.Radius})
	if cel.Station != nil {
		services := append([]string(nil), cel.Station.Services...)
		s.Stations.Set(e, Station{Services: services})
	}
	if cel.Gate != nil {
		s.Gates.Set(e, Stargate{ToSystem: cel.Gate.ToSystem, ToCel: cel.Gate.ToCel})
	}
	if cel.Belt != nil {
		// Rocks come from the persistent belt state: seeded from the def on
		// first generation, afterwards the saved (depleted/regrown) volumes.
		st := s.ensureBeltState(cel)
		for i, rock := range st.Rocks {
			s.spawnAsteroid(cel, rock, i)
		}
	}
	for _, sp := range cel.Spawns {
		s.spawnGroup(cel, sp, pending)
	}
	if cel.Anomaly != nil {
		for _, sp := range cel.Anomaly.Spawns {
			s.spawnGroup(cel, sp, pending)
		}
	}
}

// spawnGroup spawns the NPCs of one spawn group, honoring pending respawn
// timers that already owe the group members. Respawn cadence is re-derived
// from the celestial definition when a member dies, so it needs no storage
// here (it must also survive save/restore, which only persists timers).
func (s *Sim) spawnGroup(cel *content.CelestialDef, sp content.SpawnDef, pending map[string]int) {
	key := string(cel.ID) + "|" + string(sp.NpcID)
	owed := pending[key]
	for i := 0; i < sp.Count; i++ {
		if owed > 0 {
			// This member is already owed by a pending timer; don't double
			// the group. The timer itself respawns it.
			owed--
			continue
		}
		s.spawnNpc(sp.NpcID, cel.Position, celestialSpread(cel))
	}
}

// celestialSpread returns the scatter radius used around a celestial.
func celestialSpread(cel *content.CelestialDef) float64 {
	if cel.Radius > 0 {
		return cel.Radius
	}
	return 1000
}

// ensureBeltState returns the persistent state of one belt, seeding it from
// the BeltDef on first generation (every rock at its definition volume —
// today's fresh-world semantics) and reusing the saved state thereafter.
func (s *Sim) ensureBeltState(cel *content.CelestialDef) *BeltState {
	if s.belts == nil {
		s.belts = make(map[beltKey]*BeltState)
	}
	k := beltKey{System: s.systemID, Cel: cel.ID}
	if st := s.belts[k]; st != nil {
		return st
	}
	st := &BeltState{System: s.systemID, Cel: cel.ID, GrownAt: s.ticks}
	if cel.Belt != nil {
		for _, ad := range cel.Belt.Asteroids {
			for i := 0; i < ad.Count; i++ {
				st.Rocks = append(st.Rocks, BeltRock{Ore: ad.Ore, Remaining: ad.Volume, Volume: ad.Volume})
			}
		}
	}
	s.belts[k] = st
	return st
}

// spawnAsteroid scatters one rock deterministically around its belt, from
// its persistent belt-state volume.
// MASTER-STREAM role (spawn entropy): two draws per rock from s.RNG — the
// master's only scatter consumer besides spawnNpcStream; see the RNG field
// on Sim and rng_roles_test.go. The rolls are ALWAYS consumed, even when
// the rock's persisted volume is exhausted and no entity is created, so a
// depleting belt never shifts the master stream's scatter sequence.
func (s *Sim) spawnAsteroid(cel *content.CelestialDef, rock BeltRock, index int) {
	spread := celestialSpread(cel)
	angle := s.RNG.Range(0, 2*math.Pi)
	r := s.RNG.Range(0, spread)
	if rock.Remaining <= beltDepletedEpsilon {
		return // depleted: state-only until regrowth brings it back
	}
	pos := cel.Position.Add(core.Vec2{X: math.Cos(angle) * r, Y: math.Sin(angle) * r})

	name := "Asteroid"
	if def, ok := s.Content.Item(rock.Ore); ok {
		name = def.Name + " Asteroid"
	}
	e := s.create()
	s.Names.Set(e, Name{Value: name})
	s.Transforms.Set(e, Transform{Pos: pos, Facing: angle})
	s.Asteroids.Set(e, Asteroid{Ore: rock.Ore, Remaining: rock.Remaining, Belt: cel.ID, Index: index})
}

// spawnNpc creates a full NPC ship entity anchored at celPos (scattered
// within spread) with its own forked RNG stream. It returns the entity, or
// zero when the definition is missing/invalid.
// MASTER-STREAM role (spawn entropy): it passes s.RNG, so every NPC a gate
// jump, world load or respawn timer creates consumes two master rolls (see
// rng_roles_test.go); the Fork seed below only reads the master state.
func (s *Sim) spawnNpc(id content.NpcID, celPos core.Vec2, spread float64) ecs.Entity {
	return s.spawnNpcStream(id, celPos, spread, s.RNG)
}

// spawnNpcStream is spawnNpc drawing its spawn scatter from the given RNG
// stream instead of the master stream. Traffic spawns use it so convoy
// activity never consumes master-stream rolls (existing seed-pinned
// outcomes stay put).
func (s *Sim) spawnNpcStream(id content.NpcID, celPos core.Vec2, spread float64, rng *core.RNG) ecs.Entity {
	def, ok := s.Content.Npc(id)
	if !ok {
		return 0
	}
	hull, ok := s.Content.Ship(def.Ship)
	if !ok {
		return 0
	}
	fit, err := s.buildFitting(def.Fit, hull)
	if err != nil {
		return 0
	}

	angle := rng.Range(0, 2*math.Pi)
	r := rng.Range(0, spread)
	pos := celPos.Add(core.Vec2{X: math.Cos(angle) * r, Y: math.Sin(angle) * r})

	e := s.create()
	s.Names.Set(e, Name{Value: def.Name})
	s.Transforms.Set(e, Transform{Pos: pos, Facing: angle})
	s.Motions.Set(e, Motion{
		Mass: hull.Mass, MaxSpeed: hull.MaxSpeed, Agility: hull.Agility,
		SigRadius: hull.SigRadius, SpeedMultiplier: 1, WebFactor: 1,
	})
	s.Ships.Set(e, ShipState{
		Hull: hull.ID, Shield: hull.Shield, Armor: hull.Armor, Structure: hull.Structure,
		MaxShield: hull.Shield, MaxArmor: hull.Armor, MaxStructure: hull.Structure,
		Resists: hull.Resists,
	})
	s.Caps.Set(e, Capacitor{Current: hull.Capacitor, Capacity: hull.Capacitor, Tau: hull.Recharge})
	s.Cargos.Set(e, Cargo{
		Stacks:      s.npcCharges(def.Fit),
		Capacity:    hull.Cargo,
		OreCapacity: hull.OreHold,
	})
	s.Fittings.Set(e, fit)
	s.Targetings.Set(e, Targeting{
		MaxTargets: hull.MaxTargets, MaxRange: hull.MaxTargetRange, ScanRes: hull.ScanResolution,
	})
	s.AIs.Set(e, NpcAI{
		Npc: def.ID, State: NpcIdle, Home: celPos,
		// The fork derives from the master stream's state at spawn time and
		// the fresh handle, so streams are independent and reproducible.
		Rand: s.RNG.Fork(uint64(e)),
	})
	s.recomputeFit(e)
	s.healToFull(e) // plates extend the maxima; NPCs spawn at full strength
	return e
}

// npcCharges returns the ammunition an NPC spawns with: a generous stack of
// the first (sorted) item matching each fitted weapon's ammo class, so rats
// never run dry before they can respawn. Deterministic: sorted item IDs.
func (s *Sim) npcCharges(fit []content.ModuleID) []content.CargoStack {
	itemIDs := make([]content.ItemID, 0, len(s.Content.Items))
	for id := range s.Content.Items {
		itemIDs = append(itemIDs, id)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })

	var stacks []content.CargoStack
	seen := make(map[string]bool)
	for _, mid := range fit {
		def, ok := s.Content.Module(mid)
		if !ok || def.Weapon == nil || def.Weapon.AmmoClass == "" || seen[def.Weapon.AmmoClass] {
			continue
		}
		for _, id := range itemIDs {
			if item, ok := s.Content.Item(id); ok && item.AmmoClass == def.Weapon.AmmoClass {
				stacks = addStacks(stacks, id, 1000)
				seen[def.Weapon.AmmoClass] = true
				break
			}
		}
	}
	return stacks
}
