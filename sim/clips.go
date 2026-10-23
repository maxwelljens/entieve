package sim

import (
	"math"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// Shell bays ("clips"): every charge-consuming module — a projectile gun with
// an ammo class, or the cap booster — carries a fixed m³ bay that holds whole
// rounds of exactly ONE shell type. Every load takes one firing cycle: the
// rounds move at the start (take-at-start conservation) and the bay is not
// ready until the deadline, so the load DEFERS the next shot by one cycle.
// Only the fresh-fit pre-load at boot is instant (preloadBays). Each shot
// draws one round; the bay tops itself up from the selected type while that
// type has cargo stock.
//
// Selection rule (the only automatic behaviour there is):
//   - the bay loads the selected type while its cargo stock lasts;
//   - when that type runs out, the bay switches if exactly one other type
//     remains in cargo and waits if two or more compete (no cargo-order
//     following anywhere);
//   - a fresh fit (or one whose selection was cleared) does the same: one
//     type present = auto-load, two or more = sit dry and wait for a choice.
//
// Truly dry (empty bay, no matching rounds in cargo) keeps the original
// "no charges" warning and deactivates the module.

// cyclePeriod is a module's cycle length: the firing cycle for a gun, the
// activation time otherwise (1 s floor, matching the cycle systems).
func cyclePeriod(def *content.ModuleDef) float64 {
	p := def.ActivationTime
	if def.Weapon != nil && def.Weapon.ShotTime > 0 {
		p = def.Weapon.ShotTime
	}
	if p <= 0 {
		p = 1
	}
	return p
}

// clipEpsilon absorbs the binary rounding of decimal m³ volumes so a bay
// sized as an exact multiple of a shell volume reports that whole count.
const clipEpsilon = 1e-9

// clipBay is a module's bay capacity in m³ (0 = the module loads no rounds).
func clipBay(def *content.ModuleDef) float64 {
	if def.Kind == content.KindCapBooster {
		if def.Boost != nil {
			return def.Boost.BayM3
		}
		return 0
	}
	if def.Weapon != nil {
		return def.Weapon.BayM3
	}
	return 0
}

// clipRound reports whether item is a legal round for the module's bay: a
// shell of the gun's family, or the cap booster's cell.
func clipRound(def *content.ModuleDef, item *content.ItemDef) bool {
	if def.Kind == content.KindCapBooster {
		return item.ID == capCellID
	}
	return def.Weapon != nil && def.Weapon.AmmoClass != "" && item.AmmoClass == def.Weapon.AmmoClass
}

// clipRounds is the bay's capacity in whole rounds of one shell (0 when the
// shell is unknown or larger than the bay).
func (s *Sim) clipRounds(def *content.ModuleDef, item content.ItemID) int {
	round, ok := s.Content.Item(item)
	if !ok || round.Volume <= 0 {
		return 0
	}
	bay := clipBay(def)
	if bay <= 0 {
		return 0
	}
	return int(math.Floor(bay/round.Volume + clipEpsilon))
}

// clipTypes lists the distinct round types carried in cargo, sorted by id.
// Sorted (not stack order) so a bay's choice never depends on how cargo was
// stacked.
func (s *Sim) clipTypes(def *content.ModuleDef, stacks []content.CargoStack) []content.ItemID {
	seen := map[content.ItemID]bool{}
	var out []content.ItemID
	for _, st := range stacks {
		if st.Qty <= 0 || seen[st.Item] {
			continue
		}
		seen[st.Item] = true
		if item, ok := s.Content.Item(st.Item); ok && clipRound(def, item) {
			out = append(out, st.Item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// clipState is what fillClip did (or found).
type clipState int

const (
	clipKept      clipState = iota // bay loaded/loading (or no bay / nothing to load)
	clipFilled                     // a full-bay load started
	clipAmbiguous                  // empty bay, no selection, 2+ types in cargo: wait
)

// fillClip starts a load into one empty bay from cargo. A loaded or loading
// bay is never touched; a bay whose selection still has cargo stock takes
// that type, otherwise the selection clears and the type count decides (1 =
// load it, 0 = dry, 2+ = wait). A full bay's rounds move at once (take at
// start: a save mid-load can neither duplicate nor lose rounds) and the bay
// holds the load cycle until its deadline (loadClip, finishLoad).
func (s *Sim) fillClip(e ecs.Entity, def *content.ModuleDef, rt *ModuleRuntime) clipState {
	bay := clipBay(def)
	cargo, ok := s.Cargos.Get(e)
	if bay <= 0 || !ok || rt.Clip.Qty > 0 {
		return clipKept
	}
	if rt.Selection != "" {
		if stacksCount(cargo.Stacks, rt.Selection) > 0 {
			if n := s.loadClip(cargo, def, rt, rt.Selection); n > 0 {
				return clipFilled
			}
		}
		rt.Selection = "" // the selected type is out of cargo
	}
	types := s.clipTypes(def, cargo.Stacks)
	switch len(types) {
	case 0:
		return clipKept
	case 1:
		if n := s.loadClip(cargo, def, rt, types[0]); n > 0 {
			return clipFilled
		}
		return clipKept
	}
	return clipAmbiguous
}

// loadClip moves up to a full bay of item from cargo into the bay and starts
// the load: the rounds leave cargo at once, and the bay is not ready until the
// load's deadline (CycleEnds) passes. Selection is written only when the load
// completes, so a loaded bay with no selection is exactly a load in flight
// (loadInFlight) — no extra save field.
func (s *Sim) loadClip(cargo *Cargo, def *content.ModuleDef, rt *ModuleRuntime, item content.ItemID) int {
	rounds := s.clipRounds(def, item)
	if rounds <= 0 {
		return 0
	}
	stacks, taken := takeStacks(cargo.Stacks, item, rounds)
	if taken <= 0 {
		return 0
	}
	cargo.Stacks = stacks
	rt.Clip = content.CargoStack{Item: item, Qty: taken}
	rt.Selection = ""
	// A load costs one firing cycle. A longer cycle already in flight (a
	// cancelled module's clock) is kept, as CmdSetAmmo always deferred.
	if due := s.time + cyclePeriod(def); rt.CycleEnds < due {
		rt.CycleEnds = due
	}
	return taken
}

// loadInFlight reports a load whose rounds have already moved but whose
// deadline has not passed. Both halves are persistent state (the bay and the
// selection), so a save mid-load restores into the same load.
func loadInFlight(rt *ModuleRuntime) bool {
	return rt.Clip.Item != "" && rt.Clip.Qty > 0 && rt.Selection == ""
}

// finishLoad completes one bay load: the loaded type becomes the selection
// and the one line per load fires here, at the deadline.
func (s *Sim) finishLoad(e ecs.Entity, def *content.ModuleDef, rt *ModuleRuntime) {
	rt.Selection = rt.Clip.Item
	s.logLoad(e, def, rt.Clip.Item, rt.Clip.Qty)
}

// preloadBays resolves the fresh-fit pre-load at boot: fillBays starts the
// loads and this settles them at once, leaving no clock running. No UI exists
// before the first tick to show a load cycle; everything after boot is clocked.
func (s *Sim) preloadBays() {
	s.fillBays()
	for _, e := range sortedEntities(s.Fittings) {
		fit, ok := s.Fittings.Get(e)
		if !ok {
			continue
		}
		for _, class := range slotClasses {
			defs, rts := fit.Defs(class), fit.Runtimes(class)
			for i := range rts {
				if !loadInFlight(&rts[i]) {
					continue
				}
				if def, ok := s.Content.Module(defs[i]); ok {
					s.finishLoad(e, def, &rts[i])
				}
				rts[i].CycleEnds = 0
			}
		}
	}
}

// dumpClip returns the bay's rounds to the owner's cargo (unfit, assembly and
// a CmdSetAmmo switch all use it — rounds are never destroyed).
func (s *Sim) dumpClip(e ecs.Entity, rt *ModuleRuntime) {
	if rt.Clip.Item == "" || rt.Clip.Qty <= 0 {
		rt.Clip = content.CargoStack{}
		return
	}
	if cargo, ok := s.Cargos.Get(e); ok {
		cargo.Stacks = addStacks(cargo.Stacks, rt.Clip.Item, rt.Clip.Qty)
	}
	rt.Clip = content.CargoStack{}
}

// logLoad emits the one quiet line per completed load, for the player only
// (NPC bays fill silently; the log is the player's, not the swarm's).
func (s *Sim) logLoad(e ecs.Entity, def *content.ModuleDef, item content.ItemID, n int) {
	if e != s.player {
		return
	}
	name := string(item)
	if itemDef, ok := s.Content.Item(item); ok {
		name = itemDef.Name
	}
	s.emit(Event{Kind: EvModule, Actor: e,
		Text: def.Name + " loaded with " + name + " (" + itoaN(n) + " rounds)"})
}

// fillBays resolves every bay before the cycle systems run. It completes loads
// whose deadline has passed — the module cycle loop skips idle modules, so an
// idle load has to finish here — and starts a load in every IDLE empty bay.
// An ACTIVE module starts no load here: it loads at its own cycle boundary,
// where the load costs a firing cycle (see fireWeapon and the cap booster
// branch of systemModules). Every load after boot holds the bay for one cycle.
func (s *Sim) fillBays() {
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
				def, ok := s.Content.Module(defs[i])
				if !ok || clipBay(def) <= 0 {
					continue
				}
				if loadInFlight(rt) {
					if now >= rt.CycleEnds {
						s.finishLoad(e, def, rt)
					}
					continue
				}
				if rt.Active {
					continue // active modules load at their cycle boundary
				}
				s.fillClip(e, def, rt)
			}
		}
	}
}
