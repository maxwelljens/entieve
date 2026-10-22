package sim

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// maxEvents is the pending-event cap; the oldest events are dropped beyond it.
const maxEvents = 512

// warnInterval is the minimum sim seconds between repeated activation
// warnings for one module (prevents log spam at 60 Hz).
const warnInterval = 5

// capCellID is the item a cap booster's bay loads and burns per activation.
const capCellID = content.ItemID("charge-cap-cell")

// ---------------------------------------------------------------------------
// Entity bookkeeping
// ---------------------------------------------------------------------------

// create returns a new entity while keeping Sim's free-slot mirror in sync
// with the underlying world. All sim-side entity creation must go through
// this helper so that Snapshot can reproduce exact handles on Restore.
func (s *Sim) create() ecs.Entity {
	if n := len(s.free); n > 0 {
		s.free = s.free[:n-1]
	}
	return s.World.Create()
}

// destroy removes an entity while keeping the free-slot mirror in sync and
// pruning module-target bindings that reference the entity. All sim-side
// entity destruction must go through this helper.
func (s *Sim) destroy(e ecs.Entity) {
	if !s.World.Alive(e) {
		return
	}
	idx, gen := e.Index(), e.Generation()
	s.World.Destroy(e)
	s.free = append(s.free, freeSlot{idx: idx, gen: gen})
	// Deleting-only map mutation is order independent, so this stays
	// deterministic regardless of map iteration order.
	for k, t := range s.modTargets {
		if k.owner == uint64(e) || t == e {
			delete(s.modTargets, k)
		}
	}
}

// healToFull tops every layer up to its current maximum. Spawn sites call it
// after recomputeFit so ships and NPCs enter the grid at full strength.
func (s *Sim) healToFull(e ecs.Entity) {
	if ship, ok := s.Ships.Get(e); ok {
		ship.Shield = ship.MaxShield
		ship.Armor = ship.MaxArmor
		ship.Structure = ship.MaxStructure
	}
	if cap, ok := s.Caps.Get(e); ok {
		cap.Current = cap.Capacity
	}
}

// storeLists returns the entity list of every store, used to enumerate all
// live entities.
func (s *Sim) storeLists() [][]ecs.Entity {
	return [][]ecs.Entity{
		s.Names.Entities(), s.Transforms.Entities(), s.Intents.Entities(),
		s.Motions.Entities(), s.Ships.Entities(), s.Caps.Entities(),
		s.Cargos.Entities(), s.Fittings.Entities(), s.Targetings.Entities(),
		s.Pilots.Entities(), s.AIs.Entities(), s.Asteroids.Entities(),
		s.Celestials.Entities(), s.Gates.Entities(), s.Stations.Entities(),
		s.Docked.Entities(), s.Warps.Entities(), s.Wrecks.Entities(),
		s.Traffic.Entities(), s.Patrols.Entities(),
	}
}

// allEntities returns every live entity of the world, sorted by handle.
// Every system iterates in this canonical order so that behavior never
// depends on component insertion order.
func (s *Sim) allEntities() []ecs.Entity {
	seen := make(map[ecs.Entity]bool)
	var out []ecs.Entity
	for _, list := range s.storeLists() {
		for _, e := range list {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	sortEntities(out)
	return out
}

// sortEntities orders entities ascending by raw handle value.
func sortEntities(es []ecs.Entity) {
	sort.Slice(es, func(i, j int) bool { return es[i] < es[j] })
}

// sortedEntities returns the store's entities in canonical (handle) order.
func sortedEntities[T any](st *ecs.Store[T]) []ecs.Entity {
	es := st.Entities()
	sortEntities(es)
	return es
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

// emit queues an event stamped with the current sim time, dropping the
// oldest events beyond maxEvents.
func (s *Sim) emit(ev Event) {
	ev.Time = s.time
	s.events = append(s.events, ev)
	if len(s.events) > maxEvents {
		over := len(s.events) - maxEvents
		s.events = append(s.events[:0], s.events[over:]...)
	}
}

// log queues an EvtLog line with severity and actor.
func (s *Sim) log(sev Severity, actor ecs.Entity, text string) {
	s.emit(Event{Kind: EvLog, Severity: sev, Actor: actor, Text: text})
}

// ---------------------------------------------------------------------------
// Names and formatting
// ---------------------------------------------------------------------------

// nameOf returns the display name of an entity ("Unknown" when unnamed).
func (s *Sim) nameOf(e ecs.Entity) string {
	if n, ok := s.Names.Get(e); ok {
		return n.Value
	}
	return "Unknown"
}

// nounOf is nameOf but renders the player as "you" for combat-log lines.
func (s *Sim) nounOf(e ecs.Entity) string {
	if e == s.player {
		return "you"
	}
	return s.nameOf(e)
}

// euro formats an amount per docs/lore/NAMING.md (Money strings): thousands
// separators, two decimals and the € suffix — 15000 -> "15,000.00 €"; any
// amount under one euro renders as bare cents — 0.5 -> "50¢"; zero stays
// "0.00 €". Same rounding as before (FormatFloat, 2 decimals) — only the
// unit presentation is euro.
func euro(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	i := strings.IndexByte(s, '.')
	intPart, frac := s[:i], s[i:]
	if intPart == "0" && frac != ".00" { // under one euro: bare cents, no decimals
		cents := strings.TrimLeft(frac[1:], "0")
		if neg {
			return "-" + cents + "¢"
		}
		return cents + "¢"
	}
	if i > 3 {
		var b strings.Builder
		for i, c := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(c)
		}
		intPart = b.String()
	}
	if neg {
		return "-" + intPart + frac + " €"
	}
	return intPart + frac + " €"
}

// num formats a float for event text.
func num(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// ---------------------------------------------------------------------------
// Geometry helpers
// ---------------------------------------------------------------------------

// posOf returns the transform position of e.
func (s *Sim) posOf(e ecs.Entity) (core.Vec2, bool) {
	t, ok := s.Transforms.Get(e)
	if !ok {
		return core.Vec2{}, false
	}
	return t.Pos, true
}

// distBetween returns the distance between two entities (max float when
// either position is unknown).
func (s *Sim) distBetween(a, b ecs.Entity) float64 {
	pa, ok := s.posOf(a)
	if !ok {
		return math.MaxFloat64
	}
	pb, ok := s.posOf(b)
	if !ok {
		return math.MaxFloat64
	}
	return pa.Dist(pb)
}

// nearestCelestial returns the entity of the celestial closest to pos, or
// zero when the grid has none.
func (s *Sim) nearestCelestial(pos core.Vec2) ecs.Entity {
	var best ecs.Entity
	bestD := math.MaxFloat64
	for _, e := range sortedEntities(s.Celestials) {
		t, ok := s.Transforms.Get(e)
		if !ok {
			continue
		}
		if d := t.Pos.Dist2(pos); d < bestD {
			bestD, best = d, e
		}
	}
	return best
}

// celEntity returns the grid entity for the given celestial ID.
func (s *Sim) celEntity(id content.CelID) ecs.Entity {
	for _, e := range sortedEntities(s.Celestials) {
		if c, ok := s.Celestials.Get(e); ok && c.Cel == id {
			return e
		}
	}
	return 0
}

// stationEntity returns the station entity docked at by the player, or zero.
func (s *Sim) dockedStation() ecs.Entity {
	d, ok := s.Docked.Get(s.player)
	if !ok {
		return 0
	}
	for _, e := range sortedEntities(s.Stations) {
		c, ok := s.Celestials.Get(e)
		if ok && c.Cel == d.At {
			return e
		}
	}
	return 0
}

// stationAt returns the station entity owning the given celestial ID.
func (s *Sim) stationAt(cel content.CelID) ecs.Entity {
	for _, e := range sortedEntities(s.Stations) {
		if c, ok := s.Celestials.Get(e); ok && c.Cel == cel {
			return e
		}
	}
	return 0
}

// hasService reports whether the entity is a station offering svc.
func hasService(st *Station, svc string) bool {
	for _, s := range st.Services {
		if s == svc {
			return true
		}
	}
	return false
}

// firstStationID returns the first station of the system definition
// (celestials order), or "" when the system has none.
func firstStationID(sys *content.SystemDef) content.CelID {
	if sys == nil {
		return ""
	}
	for _, cel := range sys.Celestials {
		if cel.Station != nil {
			return cel.ID
		}
	}
	return ""
}

// playerDocked reports whether the player is inside a station.
func (s *Sim) playerDocked() bool { return s.Docked.Has(s.player) }

// rngFor returns the RNG stream that owns rolls for entity e: the per-NPC
// fork for NPCs, the master stream for the player.
func (s *Sim) rngFor(e ecs.Entity) *core.RNG {
	if ai, ok := s.AIs.Get(e); ok && ai.Rand != nil {
		return ai.Rand
	}
	return s.RNG
}

// ---------------------------------------------------------------------------
// Cargo helpers
// ---------------------------------------------------------------------------

// itemVolume returns the per-unit volume of an item (unknown items and
// non-items count as 0 m³: modules and hulls occupy no volume).
func (s *Sim) itemVolume(id content.ItemID) float64 {
	if d, ok := s.Content.Item(id); ok && d.Volume > 0 {
		return d.Volume
	}
	return 0
}

// stacksVolume returns the total volume of a stack slice.
func (s *Sim) stacksVolume(st []content.CargoStack) float64 {
	var v float64
	for _, st := range st {
		v += s.itemVolume(st.Item) * float64(st.Qty)
	}
	return v
}

// stacksCount returns the number of units of item in stacks.
func stacksCount(st []content.CargoStack, item content.ItemID) int {
	n := 0
	for _, s := range st {
		if s.Item == item {
			n += s.Qty
		}
	}
	return n
}

// addStacks merges qty units of item into stacks (append-order deterministic).
func addStacks(st []content.CargoStack, item content.ItemID, qty int) []content.CargoStack {
	if qty <= 0 {
		return st
	}
	for i := range st {
		if st[i].Item == item {
			st[i].Qty += qty
			return st
		}
	}
	return append(st, content.CargoStack{Item: item, Qty: qty})
}

// takeStacks removes up to qty units of item from st, returning the new
// stacks and how many units were actually removed.
func takeStacks(st []content.CargoStack, item content.ItemID, qty int) ([]content.CargoStack, int) {
	if qty <= 0 {
		return st, 0
	}
	taken := 0
	for i := range st {
		if taken >= qty {
			break
		}
		if st[i].Item != item {
			continue
		}
		t := st[i].Qty
		if t > qty-taken {
			t = qty - taken
		}
		st[i].Qty -= t
		taken += t
	}
	// Drop exhausted stacks, preserving relative order.
	out := st[:0]
	for _, s := range st {
		if s.Qty > 0 {
			out = append(out, s)
		}
	}
	return out, taken
}

// cargoSpace returns the free m³ of the given stack slice against capacity.
func (s *Sim) cargoSpace(st []content.CargoStack, capacity float64) float64 {
	return capacity - s.stacksVolume(st)
}

// oreContainer returns the stack slice and capacity that receives mined ore:
// the dedicated ore hold when the hull has one, else the general cargo.
func oreContainer(c *Cargo) (*[]content.CargoStack, float64) {
	if c.OreCapacity > 0 {
		return &c.Ore, c.OreCapacity
	}
	return &c.Stacks, c.Capacity
}

// ---------------------------------------------------------------------------
// Fitting helpers
// ---------------------------------------------------------------------------

// moduleRuntime returns the fitting and runtime for a slot reference.
func (s *Sim) moduleRuntime(e ecs.Entity, slot SlotRef) (*Fitting, *ModuleRuntime, bool) {
	fit, ok := s.Fittings.Get(e)
	if !ok {
		return nil, nil, false
	}
	var rt []ModuleRuntime
	switch slot.Class {
	case content.SlotHigh:
		rt = fit.HighRT
	case content.SlotMid:
		rt = fit.MidRT
	case content.SlotLow:
		rt = fit.LowRT
	default:
		return nil, nil, false
	}
	if slot.Index < 0 || slot.Index >= len(rt) {
		return nil, nil, false
	}
	return fit, &rt[slot.Index], true
}

// modKey addresses one fitted module's remembered target.
type modKey struct {
	owner uint64
	class string
	index int
}

// setModTarget remembers the activation target of one module.
func (s *Sim) setModTarget(e ecs.Entity, slot SlotRef, target ecs.Entity) {
	k := modKey{owner: uint64(e), class: slot.Class, index: slot.Index}
	if target.IsZero() {
		delete(s.modTargets, k)
		return
	}
	s.modTargets[k] = target
}

// modTarget returns the remembered target of one module.
func (s *Sim) modTarget(e ecs.Entity, slot SlotRef) (ecs.Entity, bool) {
	t, ok := s.modTargets[modKey{owner: uint64(e), class: slot.Class, index: slot.Index}]
	return t, ok
}

// shiftModTargets drops the binding of the module removed at removedIndex
// and re-keys bindings of higher indexes in that slot class. Collect-then-
// mutate keeps it independent of map iteration order.
func (s *Sim) shiftModTargets(owner ecs.Entity, class string, removedIndex int) {
	type move struct {
		key modKey
		tgt ecs.Entity
	}
	var (
		dels  []modKey
		moves []move
	)
	for k, tgt := range s.modTargets {
		if k.owner != uint64(owner) || k.class != class {
			continue
		}
		switch {
		case k.index == removedIndex:
			dels = append(dels, k)
		case k.index > removedIndex:
			dels = append(dels, k)
			moves = append(moves, move{key: modKey{owner: k.owner, class: k.class, index: k.index - 1}, tgt: tgt})
		}
	}
	for _, k := range dels {
		delete(s.modTargets, k)
	}
	for _, m := range moves {
		s.modTargets[m.key] = m.tgt
	}
}

// slotClasses is the canonical slot iteration order.
var slotClasses = []string{content.SlotHigh, content.SlotMid, content.SlotLow}

// landmarkKind reports whether an Info.Kind is a system landmark: the
// star, planet, moon, station, gate, belt and anomaly markers. Landmarks
// are always visible and warpable regardless of sensor range (see Info).
func landmarkKind(kind string) bool {
	switch kind {
	case KindStar, KindPlanet, KindMoon, KindStation, KindGate, KindBelt, KindAnomaly:
		return true
	}
	return false
}

// ammoPref returns the shell type a weapon slot's bay tops up from ("" =
// none: waiting for a choice, or no bay). It travels with the slot's runtime,
// so refitting and save/restore keep it in step automatically.
func (s *Sim) ammoPref(e ecs.Entity, slot SlotRef) content.ItemID {
	if _, rt, ok := s.moduleRuntime(e, slot); ok {
		return rt.Selection
	}
	return ""
}

// AmmoPref is the read-only form of ammoPref: one weapon slot's selected
// shell type ("" = none). The interface's predicted-DPS figure reads it so it
// names the shell the sim would actually load.
func (s *Sim) AmmoPref(e ecs.Entity, slot SlotRef) content.ItemID {
	return s.ammoPref(e, slot)
}

// recomputeFit recomputes all derived ship stats from the hull plus the
// fitted passive modules: layer maxes, resists, capacitor capacity/tau,
// base speed, targeting capability, cargo capacities, and the active
// afterburner speed multiplier. Current layer HP is clamped to the new maxes
// (fitting a module never repairs damage, un-fitting never destroys HP
// beyond clamping).
func (s *Sim) recomputeFit(e ecs.Entity) {
	ship, ok := s.Ships.Get(e)
	if !ok {
		return
	}
	hull, ok := s.Content.Ship(ship.Hull)
	if !ok {
		return
	}
	maxShield, maxArmor := hull.Shield, hull.Armor
	maxStruct := hull.Structure
	res := hull.Resists
	capCap, tau := hull.Capacitor, hull.Recharge
	speed := hull.MaxSpeed
	abMult := 1.0

	if fit, ok := s.Fittings.Get(e); ok {
		for _, class := range slotClasses {
			defs, rts := fit.Defs(class), fit.Runtimes(class)
			for i, id := range defs {
				def, ok := s.Content.Module(id)
				if !ok {
					continue
				}
				if p := def.Passive; p != nil {
					maxShield += p.ShieldHP
					maxArmor += p.ArmorHP
					capCap += p.Capacitor
					tau += p.Recharge
					res = res.Add(p.ResistBonus)
					speed += p.MaxSpeed
				}
				if def.Kind == content.KindAfterburner && i < len(rts) && rts[i].Active {
					if def.Boost != nil && def.Boost.SpeedMultiplier > abMult {
						abMult = def.Boost.SpeedMultiplier
					}
				}
			}
		}
	}
	// Resists stay strictly below 1 (documented clamp).
	res = content.Resists{
		EM:        clamp(res.EM, 0, 0.999),
		Thermal:   clamp(res.Thermal, 0, 0.999),
		Kinetic:   clamp(res.Kinetic, 0, 0.999),
		Explosive: clamp(res.Explosive, 0, 0.999),
	}

	// Fitting/unfitting passive HP modules adjusts the maxima only and clamps
	// current HP into range (EVE semantics: installing a plate does not heal
	// you). Spawn sites call healToFull afterwards so a fresh ship or rat
	// starts at full strength — an NPC spawning "damaged" would aggro at any
	// range.
	ship.MaxShield = maxShield
	ship.MaxArmor = maxArmor
	ship.MaxStructure = maxStruct
	ship.Resists = res
	ship.Shield = math.Min(ship.Shield, maxShield)
	ship.Armor = math.Min(ship.Armor, maxArmor)
	ship.Structure = math.Min(ship.Structure, maxStruct)

	if cap, ok := s.Caps.Get(e); ok {
		cap.Capacity = capCap
		cap.Tau = tau
		if cap.Current > capCap {
			cap.Current = capCap
		}
	}
	if m, ok := s.Motions.Get(e); ok {
		m.MaxSpeed = speed
		m.Mass = hull.Mass
		m.Agility = hull.Agility
		m.SigRadius = hull.SigRadius
		m.SpeedMultiplier = abMult
	}
	if t, ok := s.Targetings.Get(e); ok {
		t.MaxTargets = hull.MaxTargets
		t.MaxRange = hull.MaxTargetRange
		t.ScanRes = hull.ScanResolution
	}
	if c, ok := s.Cargos.Get(e); ok {
		c.Capacity = hull.Cargo
		c.OreCapacity = hull.OreHold
	}
}

// clamp bounds v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// wrapPi folds an angle difference into [-pi, pi].
func wrapPi(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// errf builds an error for New/LoadSystem/Restore failures.
func errf(format string, args ...any) error { return fmt.Errorf(format, args...) }

// sortInfos orders Info entries by distance, ties broken by handle.
func sortInfos(infos []Info) {
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Distance != infos[j].Distance {
			return infos[i].Distance < infos[j].Distance
		}
		return infos[i].Entity < infos[j].Entity
	})
}

// dockRange is the maximum distance (m) for docking, jumping and looting —
// EVE's 2,500 m session-range convention.
const dockRange = 2500
