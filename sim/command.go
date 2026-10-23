package sim

import (
	"strconv"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// runCommands drains the queue and applies each command. Movement/lock/
// module commands take effect immediately; loot, station-service and travel
// commands are deferred to their documented systems later in this tick.
func (s *Sim) runCommands() {
	q := s.queue
	s.queue = nil
	s.deferLoot = nil
	s.deferEcon = nil
	s.deferTravel = nil
	s.deferMission = nil
	s.deferIndustry = nil
	for _, cmd := range q {
		s.applyCommand(cmd)
	}
}

// applyCommand routes one command. Commands always drive the player entity;
// NPCs act through their AI instead.
func (s *Sim) applyCommand(cmd Command) {
	p := s.player
	if p.IsZero() || !s.World.Alive(p) {
		return
	}
	// While the warp drive is engaged the state machine owns movement:
	// CmdStop is ignored (warp cannot be aborted by stopping), while course
	// commands (manual/approach/orbit/align) are recorded as the intent and
	// take effect the moment the drive releases the ship on landing — they
	// never fight the align gate or the tunnel profile.
	warping := s.Warps.Has(p)

	switch c := cmd.(type) {
	case CmdStop:
		if warping {
			return
		}
		s.Intents.Set(p, Intent{Kind: IntentStop})

	case CmdManualVel:
		dir := c.Dir.Norm()
		if dir.IsZero() {
			s.Intents.Set(p, Intent{Kind: IntentStop})
			return
		}
		s.Intents.Set(p, Intent{Kind: IntentManual, Dir: dir, SpeedFrac: clamp(c.SpeedFrac, 0, 1)})

	case CmdApproach:
		if s.validMoveTarget(c.Target) {
			s.Intents.Set(p, Intent{Kind: IntentApproach, Target: c.Target})
		}

	case CmdOrbit:
		if s.validMoveTarget(c.Target) {
			s.Intents.Set(p, Intent{Kind: IntentOrbit, Target: c.Target, Range: c.Range})
		}

	case CmdAlignTo:
		if s.validMoveTarget(c.Target) {
			s.Intents.Set(p, Intent{Kind: IntentAlign, Target: c.Target})
		}

	case CmdWarpTo:
		s.beginWarp(c.Target, c.Range)

	case CmdSetAmmo:
		s.execSetAmmo(c)

	case CmdLock:
		s.cmdLock(c.Target)

	case CmdUnlock:
		if tg, ok := s.Targetings.Get(p); ok {
			kept := tg.Locks[:0]
			for _, l := range tg.Locks {
				if l.Target != c.Target {
					kept = append(kept, l)
				}
			}
			tg.Locks = kept
		}

	case CmdActivate:
		s.activate(p, c.Slot, c.Target, false)

	case CmdDeactivate:
		s.deactivate(p, c.Slot, "", SevInfo)

	case CmdMine:
		def, rt, ok := s.moduleDef(p, c.Slot)
		if !ok {
			s.log(SevWarning, p, "No such module slot")
			return
		}
		if def.Kind != content.KindMiningLaser {
			s.log(SevWarning, p, def.Name+" is not a mining laser")
			return
		}
		_ = rt
		s.activate(p, c.Slot, c.Asteroid, false)

	case CmdLoot:
		s.deferLoot = append(s.deferLoot, c)

	case CmdTransfer:
		s.execTransfer(c)

	case CmdDock, CmdUndock, CmdJump:
		s.deferTravel = append(s.deferTravel, c)

	case CmdAcceptMission, CmdAbandonMission:
		s.deferMission = append(s.deferMission, c)

	case CmdBuy, CmdSell, CmdRefine, CmdRepair, CmdAssemble, CmdFitModule, CmdUnfitModule,
		CmdPlaceOrder, CmdCancelOrder:
		s.deferEcon = append(s.deferEcon, c)

	case CmdStartJob, CmdCollectJob, CmdCancelJob:
		s.deferIndustry = append(s.deferIndustry, c)
	}
}

// holdLabel renders a CmdTransfer hold name for log and event text.
func holdLabel(hold string) string {
	switch hold {
	case "cargo":
		return "cargo hold"
	case "ore":
		return "ore hold"
	case "hangar":
		return "station hangar"
	}
	return hold
}

// KmLabel renders a metre distance as the kilometre figure the refusal
// log lines and the UI hints quote ("150 km" for WarpMinRange). Beyond
// 99,999 km it switches like theme.FormatDistance: "<0.01 AU" below
// 0.01 AU, then two-decimal AU.
func KmLabel(m float64) string {
	switch {
	case m <= 9.9999e7:
		return strconv.FormatFloat(m/1000, 'f', -1, 64) + " km"
	case m < 1.496e9:
		return "<0.01 AU"
	default:
		return strconv.FormatFloat(m/1.496e11, 'f', 2, 64) + " AU"
	}
}

// execTransfer moves qty units of an item between the player's storage holds
// (general cargo, ore hold, station hangar). It runs immediately with the
// other player commands — only the hangar side needs station context.
// The UI inventory panels drive it; validation is strict (no partial moves)
// and every rejection is a SevWarning log line.
func (s *Sim) execTransfer(c CmdTransfer) {
	p := s.player
	validHold := func(h string) bool { return h == "cargo" || h == "ore" || h == "hangar" }
	if c.From == c.To || !validHold(c.From) || !validHold(c.To) {
		s.log(SevWarning, p, "Invalid transfer holds")
		return
	}
	if c.Qty < 1 {
		s.log(SevWarning, p, "Invalid quantity")
		return
	}
	// The item must exist (items and modules are both stackable in holds).
	if _, ok := s.Content.Item(c.Item); !ok {
		if _, ok := s.Content.Module(content.ModuleID(c.Item)); !ok {
			s.log(SevWarning, p, "Unknown item "+string(c.Item))
			return
		}
	}
	cargo, ok := s.Cargos.Get(p)
	if !ok {
		s.log(SevWarning, p, "This ship has no cargo hold")
		return
	}
	// Ore hold semantics: a hull without a dedicated ore hold has no "ore".
	if (c.From == "ore" || c.To == "ore") && cargo.OreCapacity <= 0 {
		s.log(SevWarning, p, "This ship has no ore hold")
		return
	}
	// The hangar side requires being docked at a fitting-capable station.
	if c.From == "hangar" || c.To == "hangar" {
		if !s.playerDocked() {
			s.log(SevWarning, p, "Must be docked to access the hangar")
			return
		}
		if _, ok := s.stationAccess(content.SvcFitting); !ok {
			return
		}
	}

	// Source stacks and availability. Strict: Qty exceeding what is held is
	// rejected outright (partial moves would desynchronize the UI counts).
	var src *[]content.CargoStack
	switch c.From {
	case "cargo":
		src = &cargo.Stacks
	case "ore":
		src = &cargo.Ore
	case "hangar":
		src = &s.Hangar.Items
	}
	if avail := stacksCount(*src, c.Item); c.Qty > avail {
		s.log(SevWarning, p, "Not enough "+s.itemName(c.Item)+" in "+holdLabel(c.From))
		return
	}

	// Destination capacity in m³ (the hangar is unlimited).
	if c.To != "hangar" {
		vol := s.itemVolume(c.Item) * float64(c.Qty)
		free := s.cargoSpace(cargo.Stacks, cargo.Capacity)
		if c.To == "ore" {
			free = s.cargoSpace(cargo.Ore, cargo.OreCapacity)
		}
		if vol > free+1e-9 {
			s.log(SevWarning, p, "Not enough space in "+holdLabel(c.To))
			return
		}
	}

	// Move: take from the source, then merge into the destination (stacks
	// merge via addStacks; the two slices are always distinct because
	// From != To). Both accesses are in fixed order — fully deterministic.
	*src, _ = takeStacks(*src, c.Item, c.Qty)
	switch c.To {
	case "cargo":
		cargo.Stacks = addStacks(cargo.Stacks, c.Item, c.Qty)
	case "ore":
		cargo.Ore = addStacks(cargo.Ore, c.Item, c.Qty)
	case "hangar":
		s.Hangar.Items = addStacks(s.Hangar.Items, c.Item, c.Qty)
	}
	s.emit(Event{Kind: EvMarket, Severity: SevInfo, Actor: p, Item: c.Item,
		Text:  "Transferred " + itoaN(c.Qty) + " x " + s.itemName(c.Item) + " to " + holdLabel(c.To),
		Value: float64(c.Qty)})
}

// validMoveTarget reports whether a movement command target is usable and
// warns when it is not.
func (s *Sim) validMoveTarget(target ecs.Entity) bool {
	if target.IsZero() || !s.World.Alive(target) || !s.Transforms.Has(target) {
		s.log(SevWarning, s.player, "Invalid movement target")
		return false
	}
	return true
}

// moduleDef resolves the definition and runtime behind a slot reference.
func (s *Sim) moduleDef(e ecs.Entity, slot SlotRef) (*content.ModuleDef, *ModuleRuntime, bool) {
	_, rt, ok := s.moduleRuntime(e, slot)
	if !ok {
		return nil, nil, false
	}
	def, ok := s.Content.Module(rt.Def)
	if !ok {
		return nil, nil, false
	}
	return def, rt, true
}

// execSetAmmo applies a CmdSetAmmo: the slot must be a charge-fed weapon, a
// non-empty item must exist, match the weapon's shell family and sit in
// cargo, and choosing it dumps the bay back to cargo and starts a full-bay
// load of the new type immediately (the module stays Active and resumes when
// the load cycle ends). Item "" clears the selection: the bay dumps and the
// gun re-resolves by the bay rule — auto-load if exactly one type is left in
// cargo, otherwise it waits for a choice. Every rejection logs a SevWarning
// "Cannot load X into Y" and changes nothing.
func (s *Sim) execSetAmmo(c CmdSetAmmo) {
	p := s.player
	def, rt, ok := s.moduleDef(p, c.Slot)
	if !ok {
		s.log(SevWarning, p, "No such module slot")
		return
	}
	// Label for warning text: the charge's display name (raw ID when
	// unknown, "ammo" for the clear).
	label := string(c.Item)
	if c.Item == "" {
		label = "ammo"
	} else if item, ok := s.Content.Item(c.Item); ok {
		label = item.Name
	}
	cannot := func() {
		s.log(SevWarning, p, "Cannot load "+label+" into "+def.Name)
	}
	if def.Kind != content.KindWeapon || def.Weapon == nil || def.Weapon.AmmoClass == "" {
		cannot()
		return
	}
	if c.Item == "" {
		s.dumpClip(p, rt)
		rt.Selection = ""
		s.emit(Event{Kind: EvModule, Actor: p, Text: def.Name + " ammo selection cleared"})
		if s.fillClip(p, def, rt) == clipAmbiguous {
			s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: p,
				Text: def.Name + " waiting for ammo selection"})
		}
		return
	}
	item, ok := s.Content.Item(c.Item)
	if !ok || !clipRound(def, item) {
		cannot()
		return
	}
	if cargo, ok := s.Cargos.Get(p); !ok || stacksCount(cargo.Stacks, c.Item) < 1 {
		cannot()
		return
	}
	cargo, _ := s.Cargos.Get(p)
	s.dumpClip(p, rt)
	if s.loadClip(cargo, def, rt, c.Item) <= 0 {
		cannot()
	}
	// The switch takes one firing cycle, like every load: loadClip set the
	// deadline and the completion logs.
}

// cmdLock starts locking a target from the player's ship.
func (s *Sim) cmdLock(target ecs.Entity) {
	p := s.player
	if target.IsZero() || !s.World.Alive(target) || target == p {
		s.log(SevWarning, p, "Invalid lock target")
		return
	}
	tg, ok := s.Targetings.Get(p)
	if !ok {
		return
	}
	for _, l := range tg.Locks {
		if l.Target == target {
			return // already locking or locked
		}
	}
	if len(tg.Locks) >= tg.MaxTargets {
		s.log(SevWarning, p, "Maximum number of targets reached")
		return
	}
	if s.distBetween(p, target) > tg.MaxRange {
		s.log(SevWarning, p, "Target out of lock range")
		return
	}
	tg.Locks = append(tg.Locks, Lock{Target: target})
}

// beginWarp validates and starts the warp state machine toward a celestial.
//
// rangeM is the warp-to range: metres short of the target along the warp
// vector where the drive lands (0 = land on the target; negatives clamp to
// 0). A range at or beyond the current distance to the target collapses to
// 0 — you cannot land short of a target you are already that close to (and
// a landing point at the current position would stall the align phase).
// The 2500 m landing tolerance (warpLandRange) applies around the landing
// point, not the target. The drive also refuses to engage inside
// WarpMinRange (150 km) of the target at command time.
func (s *Sim) beginWarp(target ecs.Entity, rangeM float64) {
	p := s.player
	if s.playerDocked() {
		s.log(SevWarning, p, "Cannot warp while docked")
		return
	}
	if s.Warps.Has(p) {
		return
	}
	if target.IsZero() || !s.World.Alive(target) || target == p {
		s.log(SevWarning, p, "Invalid warp destination")
		return
	}
	// Only celestial-tagged grid objects are warp destinations (CanWarp).
	if _, ok := s.Celestials.Get(target); !ok {
		s.log(SevWarning, p, "Cannot warp there")
		return
	}
	// EVE's minimum warp range: the drive cannot engage a target the
	// ship is already inside 150 km of (distance at command time).
	if d := s.distBetween(p, target); d < WarpMinRange {
		s.log(SevWarning, p, "Target within warp minimum range ("+KmLabel(WarpMinRange)+")")
		return
	}
	if sh, ok := s.Ships.Get(p); ok && sh.ScrambledUntil > s.time {
		s.emit(Event{Kind: EvWarp, Phase: "scrammed", Severity: SevWarning, Actor: p,
			Text: "Warp scrambled!"})
		return
	}
	tp, ok := s.posOf(target)
	if !ok {
		return
	}
	// Resolve the landing point: rangeM metres short of the target along
	// the player -> target warp vector.
	if rangeM < 0 {
		rangeM = 0
	}
	land := tp
	if pp, ok := s.posOf(p); ok && rangeM > 0 {
		if dir := tp.Sub(pp); dir.IsZero() {
			rangeM = 0 // already on top of the target
		} else if d := dir.Len(); rangeM >= d {
			rangeM = 0 // cannot land short of a target closer than the range
		} else {
			land = tp.Sub(dir.Norm().Scale(rangeM))
		}
	}
	s.Warps.Set(p, WarpDrive{Phase: "align", To: land})
	s.Intents.Set(p, Intent{Kind: IntentAlign, Target: target})
}
