package sim

import (
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
	// Piloting commands are ignored while the warp drive is engaged (the
	// warp state machine owns movement until it lands).
	warping := s.Warps.Has(p)

	switch c := cmd.(type) {
	case CmdStop:
		if warping {
			return
		}
		s.Intents.Set(p, Intent{Kind: IntentStop})

	case CmdManualVel:
		if warping {
			return
		}
		dir := c.Dir.Norm()
		if dir.IsZero() {
			s.Intents.Set(p, Intent{Kind: IntentStop})
			return
		}
		s.Intents.Set(p, Intent{Kind: IntentManual, Dir: dir, SpeedFrac: clamp(c.SpeedFrac, 0, 1)})

	case CmdApproach:
		if warping {
			return
		}
		if s.validMoveTarget(c.Target) {
			s.Intents.Set(p, Intent{Kind: IntentApproach, Target: c.Target})
		}

	case CmdOrbit:
		if warping {
			return
		}
		if s.validMoveTarget(c.Target) {
			s.Intents.Set(p, Intent{Kind: IntentOrbit, Target: c.Target, Range: c.Range})
		}

	case CmdAlignTo:
		if warping {
			return
		}
		if s.validMoveTarget(c.Target) {
			s.Intents.Set(p, Intent{Kind: IntentAlign, Target: c.Target})
		}

	case CmdWarpTo:
		s.beginWarp(c.Target)

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

	case CmdBuy, CmdSell, CmdRefine, CmdRepair, CmdAssemble, CmdFitModule, CmdUnfitModule:
		s.deferEcon = append(s.deferEcon, c)
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
		if _, ok := s.stationAccess("fitting"); !ok {
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
func (s *Sim) beginWarp(target ecs.Entity) {
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
	if sh, ok := s.Ships.Get(p); ok && sh.ScrambledUntil > s.time {
		s.emit(Event{Kind: EvWarp, Phase: "scrammed", Severity: SevWarning, Actor: p,
			Text: "Warp scrambled!"})
		return
	}
	tp, ok := s.posOf(target)
	if !ok {
		return
	}
	s.Warps.Set(p, WarpDrive{Phase: "align", To: tp})
	s.Intents.Set(p, Intent{Kind: IntentAlign, Target: target})
}
