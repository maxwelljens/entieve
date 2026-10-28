package sim

import (
	"math"
	"sort"
	"strconv"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

const (
	// beltDepletedEpsilon: a rock at or below this volume is destroyed
	// (mining and requisition draws), never respawned on reload and never
	// a draw source again until the next cycle refill lifts it back above.
	beltDepletedEpsilon = 1e-9
	// beltCycleTicks is one full belt cycle (216,000 ticks = 60 sim
	// minutes at 60 tps): at every boundary the whole belt refills to its
	// definition volume — the approved "fields refill on a fixed cycle"
	// rule. Pure math, zero RNG.
	beltCycleTicks = int64(216_000)
)

// systemMining runs completed cycles of active mining lasers.
//
// Each cycle moves min(VolumePerCycle, Remaining) m³ of asteroid volume into
// the ore hold (or the general cargo when the hull has no ore hold),
// converting m³ to whole units via ItemDef.Volume. Fractional m³ accumulates
// in Cargo.Remainder until it materializes as a unit, so no ore is lost to
// rounding. A hold with no room for even one whole unit emits EvtCargoFull
// and deactivates the laser; a depleted asteroid is destroyed with a log.
//
// The belt refill pass runs first: every belt state (all systems) refills
// at its 60-minute cycle boundary (see systemBelts), so mining decrements
// and the refill reset stay in step with the live asteroid entities.
func (s *Sim) systemMining() {
	s.systemBelts()
	now := s.time
	for _, e := range sortedEntities(s.Fittings) {
		fit, ok := s.Fittings.Get(e)
		if !ok {
			continue
		}
		for _, class := range slotClasses {
			defs, rts := fit.Defs(class), fit.Runtimes(class)
			for i := range rts {
				rt := &rts[i]
				if !rt.Active || now < rt.CycleEnds {
					continue
				}
				def, ok := s.Content.Module(defs[i])
				if !ok || def.Kind != content.KindMiningLaser || def.Mining == nil {
					continue
				}
				slot := SlotRef{Class: class, Index: i}
				if reason, ok := s.checkCycle(e, def, slot); !ok {
					// NPC miners go quiet: the reason line is player-only
					// noise (the EvModule deactivation still fires).
					if e != s.player {
						reason = ""
					}
					s.deactivate(e, slot, reason, SevWarning)
					continue
				}
				s.payCap(e, def.ActivationCost)
				s.mineCycle(e, def, slot)
				period := def.Mining.CycleTime
				if period <= 0 {
					period = def.ActivationTime
				}
				if period <= 0 {
					period = 1
				}
				rt.CycleEnds = now + period
			}
		}
	}
}

// mineCycle resolves one mining cycle: volume accounting, unit conversion,
// cargo-full handling and asteroid depletion.
func (s *Sim) mineCycle(e ecs.Entity, def *content.ModuleDef, slot SlotRef) {
	tgt, ok := s.modTarget(e, slot)
	if !ok {
		return
	}
	ast, ok := s.Asteroids.Get(tgt)
	if !ok {
		return
	}
	cargo, ok := s.Cargos.Get(e)
	if !ok {
		return
	}
	m := def.Mining
	// NPC miners run the same cycle machinery but keep the player's log
	// clean: no per-cycle line, no hold-full warning, no depletion line.
	quiet := e != s.player

	container, capM3 := oreContainer(cargo)
	space := capM3 - s.stacksVolume(*container)
	iv := s.mineralUnitVolume(ast.Ore)
	unitsCap := math.Floor(space / iv)
	if unitsCap <= 0 {
		// Not even one whole unit fits.
		hold := "Cargo hold"
		if cargo.OreCapacity > 0 {
			hold = "Ore hold"
		}
		if !quiet {
			s.emit(Event{Kind: EvCargoFull, Severity: SevWarning, Actor: e, Target: tgt,
				Text: hold + " full: cannot store " + s.oreName(ast.Ore)})
		}
		reason := "hold full"
		if quiet {
			reason = ""
		}
		s.deactivate(e, slot, reason, SevWarning)
		return
	}

	volWant := math.Min(m.VolumePerCycle, ast.Remaining)
	if volWant <= 0 {
		reason := "asteroid depleted"
		if quiet {
			reason = ""
		}
		s.deactivate(e, slot, reason, SevWarning)
		return
	}
	unitsNat := math.Floor((cargo.Remainder + volWant) / iv)
	units := math.Min(unitsNat, unitsCap)
	vol := volWant
	if units < unitsNat {
		// Only mine what fits as whole units this cycle.
		vol = units*iv - cargo.Remainder
		if vol < 0 {
			vol = 0
		}
	}

	// Apply: asteroid volume -> remainder/units in the ore container.
	ast.Remaining -= vol
	s.syncBeltRock(ast)
	total := cargo.Remainder + vol
	units = math.Floor(total / iv)
	cargo.Remainder = total - units*iv
	if units > 0 {
		*container = addStacks(*container, ast.Ore, int(units))
	}

	// A completed mining cycle counts as recent player activity for the
	// NPC aggro trigger (see systemNpc) and, when it put ore in the hold,
	// as progress toward mine contracts (see systemMission).
	if e == s.player {
		if tr, ok := s.Transforms.Get(e); ok {
			s.recordPlayerWork(tr.Pos)
		}
		if vol > 0 {
			s.pendingOre = append(s.pendingOre, pendingOreYield{item: ast.Ore, vol: vol})
		}
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e, Target: tgt,
			Text:  "Mined " + formatM3(vol) + " m³ " + s.oreName(ast.Ore),
			Value: vol, Item: ast.Ore})
	}

	// Depletion.
	if ast.Remaining <= beltDepletedEpsilon {
		if !quiet {
			s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e, Target: tgt,
				Text: s.nameOf(tgt) + " depleted", Item: ast.Ore})
		}
		s.destroy(tgt)
	}
}

// mineralUnitVolume returns the m³ per unit of an ore (fallback 1 for
// content without a volume).
func (s *Sim) mineralUnitVolume(ore content.ItemID) float64 {
	if d, ok := s.Content.Item(ore); ok && d.Volume > 0 {
		return d.Volume
	}
	return 1
}

// oreName returns the display name of an ore item.
func (s *Sim) oreName(ore content.ItemID) string {
	if d, ok := s.Content.Item(ore); ok {
		return d.Name
	}
	return string(ore)
}

// formatM3 renders a volume for event text: whole numbers without decimals.
func formatM3(v float64) string {
	if v == math.Floor(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// ---------------------------------------------------------------------------
// Persistent belt state — depletion, cycle refill, requisition draws
// ---------------------------------------------------------------------------

// beltKeys returns every belt-state key in canonical order (system id
// ascending, then celestial id ascending) — the iteration order for
// cycle refill and the save serialization (map order never reaches state).
func (s *Sim) beltKeys() []beltKey {
	keys := make([]beltKey, 0, len(s.belts))
	for k := range s.belts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].System != keys[j].System {
			return keys[i].System < keys[j].System
		}
		return keys[i].Cel < keys[j].Cel
	})
	return keys
}

// systemBelts refills every belt at its 60-minute cycle boundary: each rock
// resets to its definition volume, in state and on the grid. Pure math plus
// spawnAsteroid's existing master-stream scatter (no new RNG role), applied
// to every system's state so belts keep refilling while the player is
// elsewhere.
//
// The cycle is derived from the tick count alone: n := (ticks - GrownAt) /
// beltCycleTicks; GrownAt is the anchor of the last boundary (the persisted
// field, unchanged on the wire). A live rock tops up IN PLACE — the entity
// is kept, so the player's mining target survives the boundary — and a
// destroyed rock re-enters the grid through spawnAsteroid, which always
// rolls its two scatter values first (see its MASTER-STREAM note), so a
// refill can never shift the stream: the scatter is simply fresh. Off-system
// belts refill state-only; their rocks get their fresh scatter on the next
// system load, as before.
func (s *Sim) systemBelts() {
	sys := s.System()
	for _, k := range s.beltKeys() {
		st := s.belts[k]
		n := (s.ticks - st.GrownAt) / beltCycleTicks
		if n <= 0 {
			continue // epoch check first: nothing to do inside the cycle
		}
		st.GrownAt += n * beltCycleTicks
		// The loaded belt's def, for putting destroyed rocks back on the
		// grid. Off-system belts refill state-only (nil here).
		var cel *content.CelestialDef
		if k.System == s.systemID && sys != nil {
			if c := sysCelestial(sys, st.Cel); c != nil && c.Belt != nil {
				cel = c
			}
		}
		for i := range st.Rocks {
			rock := &st.Rocks[i]
			rock.Remaining = rock.Volume
			if cel == nil {
				continue // state-only: the grid rebuild scatters it
			}
			if e := s.beltRockEntity(st, i); !e.IsZero() {
				if ast, ok := s.Asteroids.Get(e); ok {
					ast.Remaining = rock.Volume // top up in place, entity kept
				}
				continue
			}
			// Destroyed rock: back on the grid at a fresh scatter.
			s.spawnAsteroid(cel, *rock, i)
		}
	}
}

// beltRockEntity returns the live asteroid entity of one belt-state slot
// (zero when the rock is not on the current grid).
func (s *Sim) beltRockEntity(st *BeltState, index int) ecs.Entity {
	for _, e := range sortedEntities(s.Asteroids) {
		if a, ok := s.Asteroids.Get(e); ok && a.Belt == st.Cel && a.Index == index {
			return e
		}
	}
	return 0
}

// syncBeltRock mirrors a live asteroid entity's volume into its belt-state
// slot (entity -> state), keeping both sides equal while the rock is on
// the grid. Rocks bound before the state existed (Belt "") are skipped.
func (s *Sim) syncBeltRock(ast *Asteroid) {
	if ast.Belt == "" || s.belts == nil {
		return
	}
	st := s.belts[beltKey{System: s.systemID, Cel: ast.Belt}]
	if st == nil || ast.Index < 0 || ast.Index >= len(st.Rocks) {
		return
	}
	st.Rocks[ast.Index].Remaining = ast.Remaining
}

// beltOreAvailable reports whether a system's belt state holds at least
// need m³ in total — the dry run before drawBeltOre, so a requisition that
// cannot be fully funded never drains the belt.
func (s *Sim) beltOreAvailable(sys content.SystemID, need float64) bool {
	if need <= beltDepletedEpsilon {
		return true
	}
	have := 0.0
	for _, k := range s.beltKeys() {
		if k.System != sys {
			continue
		}
		st := s.belts[k]
		for i := range st.Rocks {
			have += st.Rocks[i].Remaining
			if have >= need {
				return true
			}
		}
	}
	return have >= need
}

// drawBeltOre removes need m³ of ore from a system's belt state in
// canonical order (celestial id, then rock index), mirrors every live rock
// it touches and destroys one it empties (with the usual "depleted" log).
// Callers must pass beltOreAvailable first: the draw is all-or-nothing, so
// a failed requisition leaves the belt untouched. Pure math, no RNG.
func (s *Sim) drawBeltOre(sys content.SystemID, need float64) bool {
	if need <= beltDepletedEpsilon {
		return true
	}
	for _, k := range s.beltKeys() {
		st := s.belts[k]
		if k.System != sys {
			continue
		}
		for i := range st.Rocks {
			if need <= beltDepletedEpsilon {
				return true
			}
			rock := &st.Rocks[i]
			if rock.Remaining <= beltDepletedEpsilon {
				continue
			}
			take := math.Min(need, rock.Remaining)
			rock.Remaining -= take
			need -= take
			if st.System != s.systemID {
				continue
			}
			e := s.beltRockEntity(st, i)
			if e.IsZero() {
				continue
			}
			if ast, ok := s.Asteroids.Get(e); ok {
				ast.Remaining = rock.Remaining
				if ast.Remaining <= beltDepletedEpsilon {
					s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: s.celEntity(st.Cel),
						Text: s.nameOf(e) + " depleted", Item: rock.Ore})
					s.destroy(e)
				}
			}
		}
	}
	return need <= beltDepletedEpsilon
}
