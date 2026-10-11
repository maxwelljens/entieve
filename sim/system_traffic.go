package sim

import (
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Living-universe calibration constants — the binding derivation tables live
// in docs/CONTENT_AUTHORING.md ("traffic calibration contract"). Every value
// here is pinned by sim/traffic_test.go.
//
//	σ (security status)   high = 1, low = 0.5, null = 0
//	spawn interval        authored x 4^(1-σ)          -> x1, x2, x4
//	per-leg attack chance (1-σ)/3                     -> 0, 1/6, 1/3
//	price band             0.05 x 4^(1-σ)             -> ±5%, ±10%, ±20%
//	drift step             share x band / 2.5 (x2 for a lost shipment)
//	decay                  premium halved every marketDecayTicks
//	patrol cadence         authored / σ (σ = 0: no patrols)
//	patrol response        delay 1800x(1-σ) ticks, wing floor(3σ)
// ---------------------------------------------------------------------------

const (
	// trafficStreamID keys the forked living-universe RNG stream (master
	// state XOR splitMix64(id)): convoys and patrols never consume
	// master-stream rolls, so existing seed-pinned outcomes stay put.
	trafficStreamID uint64 = 0x7472_6166_6669_6301

	// trafficMaxLive caps live convoy haulers per system; trafficMaxPatrols
	// caps patrol ships. Spawn attempts beyond the cap are skipped (the
	// schedule itself is stateless, so skips never shift future attempts).
	trafficMaxLive    = 6
	trafficMaxPatrols = 4
	trafficMaxRaiders = 3
	trafficMinRaiders = 1

	// trafficFleeDelaySec is how long an ambushed hauler burns away from its
	// attackers before attempting the warp-out. Calibrated against the
	// local raiders' lock time (~15 s to lock a sig-100 hauler: a scrambler
	// or web is already active when the warp-out starts, so low-sec haulers
	// get pinned until the patrol responds or they die); trafficWarpRetrySec
	// is the cooldown between warp (re)attempts while scrambled.
	trafficFleeDelaySec = 20.0
	trafficWarpRetrySec = 3.0

	// trafficAmbushSpread is the scatter radius (m) around the ambushed
	// hauler where raiders drop in; trafficRaidersLeaveSec is how long they
	// linger after the incident resolves before breaking off.
	trafficAmbushSpread    = 3000.0
	trafficRaidersLeaveSec = 20.0

	// Traffic patrol anchors sit east of the system's first station (25 km
	// ± 5 km) — far enough that a pilot sitting at the station is never
	// inside the NPC guard radius (8 km), close enough to read as station
	// security. Engage range keeps raiders inside rail-turret optimal range.
	trafficPatrolAnchorOffset  = 25000.0
	trafficPatrolAnchorScatter = 5000.0
	trafficPatrolScatter       = 2000.0
	trafficPatrolEngageRange   = 12000.0

	// trafficBandBase is the high-sec price band: half of the market's own
	// round-trip friction (the 10% sell cut, sellRatio 0.9) — beyond that a
	// delivery could be arbitured. Bands widen with insecurity via 4^(1-σ).
	trafficBandBase = 0.05
	// trafficDriftDiv scales one full-shelf drift step to band/2.5 (2% in
	// high-sec); share scales it down for deliveries that only nibble the
	// shelf. Attack steps are double supply steps.
	trafficDriftDiv = 2.5

	// marketDecayTicks is the halving schedule: every 60 sim seconds a
	// deviating price closes half the remaining distance to its base.
	marketDecayTicks = 3600
)

// ---------------------------------------------------------------------------
// Security math (the calibration contract)
// ---------------------------------------------------------------------------

// securitySigma maps a system's Security rating onto the numeric security
// status σ used by every living-universe derivation: high = 1 (fully
// secured), low = 0.5 (contested), null = 0 (lawless). Unknown ratings
// behave like "low" (content validation pins the domain anyway).
func securitySigma(security string) float64 {
	switch security {
	case "high":
		return 1
	case "null":
		return 0
	default:
		return 0.5
	}
}

// trafficSpawnInterval scales an authored route/patrol cadence for the
// system's security: 4^(1-σ) => x1 high, x2 low, x4 null (busy, thin, rare).
func trafficSpawnInterval(interval int64, security string) int64 {
	return int64(math.Round(float64(interval) * math.Pow(4, 1-securitySigma(security))))
}

// trafficAttackChance is the per-transit-leg ambush probability (1-σ)/3:
// 0 in high-sec, 1/6 in low-sec, 1/3 in null.
func trafficAttackChance(security string) float64 {
	return (1 - securitySigma(security)) / 3
}

// trafficBand is the max |price/base − 1| allowed at a system's market:
// 0.05 x 4^(1-σ) => ±5% high, ±10% low, ±20% null.
func trafficBand(security string) float64 {
	return trafficBandBase * math.Pow(4, 1-securitySigma(security))
}

// trafficPatrolInterval scales the authored patrol cadence by 1/σ (police
// density tracks security). ok is false in null space, where there are no
// patrols at all.
func trafficPatrolInterval(interval int64, security string) (int64, bool) {
	sg := securitySigma(security)
	if sg <= 0 {
		return 0, false
	}
	return int64(math.Round(float64(interval) / sg)), true
}

// trafficResponseDelay is the ambush response lag in ticks: 1800x(1-σ) =>
// immediate in high-sec, 15 s in low-sec. (Null never responds.)
func trafficResponseDelay(security string) int64 {
	return int64(math.Round(1800 * (1 - securitySigma(security))))
}

// trafficPatrolWing is the response wing size: floor(3σ) => 3 high, 1 low,
// 0 null.
func trafficPatrolWing(security string) int {
	return int(3 * securitySigma(security))
}

// ---------------------------------------------------------------------------
// Stateless spawn schedules
// ---------------------------------------------------------------------------

// trafficPhase maps a schedule key to a deterministic offset in [0, 2^64)
// through FNV-1a plus the SplitMix64 finalizer — a pure hash of the key,
// drawing nothing from any RNG stream. Offsets stagger routes so several
// legs never fire on the same tick.
func trafficPhase(key string) uint64 {
	h := uint64(fnvOffset)
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= uint64(fnvPrime)
	}
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return h
}

// trafficDue reports whether the schedule named by key fires on the current
// tick: (ticks + phase(key) mod interval) divisible by interval. The
// schedule is stateless — no timers to persist, and skipping an attempt
// (spawn cap, no patrol) never reschedules anything.
func trafficDue(ticks int64, key string, interval int64) bool {
	if interval <= 0 {
		return false
	}
	off := int64(trafficPhase(key) % uint64(interval))
	return (ticks+off)%interval == 0
}

// ---------------------------------------------------------------------------
// The system
// ---------------------------------------------------------------------------

// systemTraffic advances the living-universe layer: convoy haulers spawn on
// their security-scaled schedules and fly one-way legs through the warp
// profile; each tunnel entry rolls the per-leg ambush chance; live ambushes
// drive raiders (real lock/activate/turret/damage pipeline), dispatch the
// patrol response, and resolve into destroyed / escaped / saved outcomes;
// deliveries and losses move station market stock and prices inside the
// security band; premiums decay halving every marketDecayTicks.
//
// Runs after every other system: warp landings, deaths and NPC state are
// already settled for the tick, and any intents set here fly next tick's
// movement pass.
func (s *Sim) systemTraffic() {
	sys := s.System()
	if sys == nil {
		return
	}
	var td *content.TrafficDef
	if t, ok := s.Content.TrafficFor(sys.ID); ok {
		td = t
	}
	sec := sys.Security
	if td != nil {
		s.trafficSpawnHaulers(sys, td, sec)
		s.trafficSpawnPatrol(sys, td, sec)
	}
	s.trafficDriveHaulers(sys, td, sec)
	s.trafficDriveAttacks(sys, td, sec)
	s.trafficDrivePatrols()
	s.trafficExpireLeaves()
	s.marketDecay()
}

// trafficExpireLeaves runs scheduled raider break-offs: at the booked time
// the raider disengages ("breaks off") and leaves the grid. Entries whose
// ship is already dead are dropped.
func (s *Sim) trafficExpireLeaves() {
	if len(s.trafficLeaves) == 0 {
		return
	}
	kept := s.trafficLeaves[:0]
	for _, l := range s.trafficLeaves {
		if !s.World.Alive(l.Entity) {
			continue
		}
		if s.time >= l.At {
			if n, ok := s.Names.Get(l.Entity); ok {
				s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: l.Entity,
					Text: n.Value + " breaks off and withdraws"})
			}
			s.destroy(l.Entity)
			continue
		}
		kept = append(kept, l)
	}
	s.trafficLeaves = kept
}

// ---------------------------------------------------------------------------
// Spawning
// ---------------------------------------------------------------------------

// trafficSpawnHaulers fires each route's spawn attempt when its schedule is
// due and the live cap allows.
func (s *Sim) trafficSpawnHaulers(sys *content.SystemDef, td *content.TrafficDef, sec string) {
	if s.Traffic.Len() >= trafficMaxLive {
		return
	}
	for i, r := range td.Routes {
		key := string(sys.ID) + "|route|" + itoaN(i)
		if !trafficDue(s.ticks, key, trafficSpawnInterval(r.IntervalTicks, sec)) {
			continue
		}
		if s.Traffic.Len() >= trafficMaxLive {
			return
		}
		s.spawnTrafficHauler(sys, td, r)
	}
}

// spawnTrafficHauler creates one convoy hauler at the route's origin
// celestial and kicks off its leg through the warp profile.
func (s *Sim) spawnTrafficHauler(sys *content.SystemDef, td *content.TrafficDef, r content.RouteDef) {
	hull, ok := s.Content.Ship(r.Hull)
	if !ok {
		return
	}
	from := sysCelestial(sys, r.From)
	to := sysCelestial(sys, r.To)
	if from == nil || to == nil {
		return
	}
	spread := from.Radius
	if spread < 1000 {
		spread = 1000
	}
	angle := s.trafficRNG.Range(0, 2*math.Pi)
	rad := s.trafficRNG.Range(0, spread)
	pos := from.Position.Add(core.Vec2{X: math.Cos(angle) * rad, Y: math.Sin(angle) * rad})

	e := s.create()
	s.Names.Set(e, Name{Value: hull.Name})
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
		Stacks:      []content.CargoStack{{Item: r.Cargo.Item, Qty: r.Cargo.Qty}},
		Capacity:    hull.Cargo,
		OreCapacity: hull.OreHold,
	})
	s.Targetings.Set(e, Targeting{
		MaxTargets: hull.MaxTargets, MaxRange: hull.MaxTargetRange, ScanRes: hull.ScanResolution,
	})
	s.Traffic.Set(e, Traffic{
		Origin: r.From, Dest: r.To, Item: r.Cargo.Item, Qty: r.Cargo.Qty,
		Phase: TrafficTransit,
	})
	s.recomputeFit(e)
	s.healToFull(e)
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDeparted, Severity: SevInfo, Actor: e,
		System: sys.ID, Cel: r.From, Item: r.Cargo.Item, Value: float64(r.Cargo.Qty),
		Text: "Convoy hauler departs " + from.Name + " bound for " + to.Name})
}

// trafficSpawnPatrol fires the patrol schedule: one guard ship per attempt,
// rotating through the configured hull palette by attempt index. Null
// security never spawns patrols.
func (s *Sim) trafficSpawnPatrol(sys *content.SystemDef, td *content.TrafficDef, sec string) {
	if td.Patrol == nil || len(td.Patrol.Hulls) == 0 {
		return
	}
	interval, ok := trafficPatrolInterval(td.Patrol.IntervalTicks, sec)
	if !ok {
		return
	}
	if s.Patrols.Len() >= trafficMaxPatrols {
		return
	}
	key := string(sys.ID) + "|patrol"
	if !trafficDue(s.ticks, key, interval) {
		return
	}
	idx := int((s.ticks / interval) % int64(len(td.Patrol.Hulls)))
	if e := s.spawnPatrolShip(sys, td.Patrol.Hulls[idx]); !e.IsZero() {
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e, System: sys.ID,
			Text: "Patrol on duty near " + patrolAnchorName(sys)})
	}
}

// patrolAnchor returns the position and celestial ID patrol wings operate
// from: the system's first station, else its first gate. The anchor point
// itself is offset east of the landmark (see trafficPatrolAnchorOffset).
func patrolAnchor(sys *content.SystemDef) (core.Vec2, content.CelID, string) {
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		if cel.Kind == content.CelStation {
			return cel.Position, cel.ID, cel.Name
		}
	}
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		if cel.Kind == content.CelGate {
			return cel.Position, cel.ID, cel.Name
		}
	}
	return core.Vec2{}, "", ""
}

func patrolAnchorName(sys *content.SystemDef) string {
	_, _, name := patrolAnchor(sys)
	return name
}

// spawnPatrolShip creates one security patrol ship at the system's patrol
// anchor (offset + scatter, drawn from the traffic stream) with the guard
// loadout registered for the hull.
func (s *Sim) spawnPatrolShip(sys *content.SystemDef, hull content.ShipID) ecs.Entity {
	def, ok := s.Content.GuardFor(hull)
	if !ok {
		return 0
	}
	base, _, _ := patrolAnchor(sys)
	if base == (core.Vec2{}) {
		return 0
	}
	dx := s.trafficRNG.Range(trafficPatrolAnchorOffset-trafficPatrolAnchorScatter,
		trafficPatrolAnchorOffset+trafficPatrolAnchorScatter)
	dy := s.trafficRNG.Range(-trafficPatrolAnchorScatter, trafficPatrolAnchorScatter)
	anchor := base.Add(core.Vec2{X: dx, Y: dy})

	e := s.spawnNpcStream(def.ID, anchor, trafficPatrolScatter, s.trafficRNG)
	if e.IsZero() {
		return 0
	}
	s.Patrols.Set(e, Patrol{State: PatrolIdle})
	return e
}

// ---------------------------------------------------------------------------
// Hauler lifecycle
// ---------------------------------------------------------------------------

// trafficDriveHaulers runs every live convoy hauler: burn away during the
// ambush window, roll the per-leg ambush at tunnel entry, mark escapes,
// deliver (or despawn at a gate/belt) on arrival, and (re)issue the leg
// warp on cooldown.
func (s *Sim) trafficDriveHaulers(sys *content.SystemDef, td *content.TrafficDef, sec string) {
	for _, e := range sortedEntities(s.Traffic) {
		tc, ok := s.Traffic.Get(e)
		if !ok {
			continue
		}
		// Ambush burn-away window: the hauler runs from its attackers until
		// FleeAt, then attempts the warp-out.
		if atk := tc.Attack; atk != nil && atk.Outcome == "" && s.time < atk.FleeAt {
			s.trafficFleeBurn(e, atk)
			continue
		}
		if wd, ok := s.Warps.Get(e); ok {
			switch {
			case wd.Phase == "warp" && !tc.Rolled:
				// First tunnel entry: the per-leg ambush roll.
				tc.Rolled = true
				s.trafficRollAmbush(e, tc, td, sec)
			case wd.Phase == "warp" && tc.Attack != nil && tc.Attack.Outcome == "":
				// A tunnel started while the ambush runs: it got away.
				s.trafficEscaped(e, tc)
			}
			continue
		}
		// Not in warp: arrived, or due for a (re)attempt.
		dest := s.celEntity(tc.Dest)
		if dest.IsZero() {
			s.destroy(e) // destination vanished (shouldn't happen): drop the leg
			continue
		}
		if s.distBetween(e, dest) <= warpLandRange {
			s.trafficArrive(sys, e, tc)
			continue
		}
		if s.time >= tc.Next {
			if dp, ok := s.posOf(dest); ok && s.warpToPoint(e, dp) {
				tc.Next = s.time + trafficWarpRetrySec
			}
		}
	}
}

// trafficFleeBurn steers an ambushed hauler directly away from its nearest
// live attacker until the warp-out window opens.
func (s *Sim) trafficFleeBurn(e ecs.Entity, atk *TrafficAttack) {
	tr, ok := s.Transforms.Get(e)
	if !ok {
		return
	}
	var away core.Vec2
	found := false
	for _, r := range atk.Raiders {
		if !s.World.Alive(r) {
			continue
		}
		if rp, ok := s.posOf(r); ok {
			away, found = tr.Pos.Sub(rp), true
			break
		}
	}
	if !found {
		s.Intents.Set(e, Intent{Kind: IntentStop})
		return
	}
	if away.IsZero() {
		away = core.Vec2{X: 1}
	}
	s.Intents.Set(e, Intent{Kind: IntentManual, Dir: away.Norm(), SpeedFrac: 1})
}

// trafficRollAmbush consumes the per-leg ambush roll when the hauler enters
// the warp tunnel (mid-leg). High security never rolls (chance 0 draws
// nothing from the stream).
func (s *Sim) trafficRollAmbush(e ecs.Entity, tc *Traffic, td *content.TrafficDef, sec string) {
	if td == nil || td.Raider == "" {
		return
	}
	p := trafficAttackChance(sec)
	if p <= 0 || !s.trafficRNG.Chance(p) {
		return
	}
	s.startAmbush(e, tc, td, sec)
}

// startAmbush drops 1-3 raiders of the system's local rat class onto the
// hauler mid-leg, interrupts its warp, and schedules the patrol response.
func (s *Sim) startAmbush(e ecs.Entity, tc *Traffic, td *content.TrafficDef, sec string) {
	def, ok := s.Content.Npc(td.Raider)
	if !ok {
		return
	}
	pos, ok := s.posOf(e)
	if !ok {
		return
	}
	n := trafficMinRaiders + s.trafficRNG.Intn(trafficMaxRaiders-trafficMinRaiders+1)
	raiders := make([]ecs.Entity, 0, n)
	for i := 0; i < n; i++ {
		r := s.spawnNpcStream(td.Raider, pos, trafficAmbushSpread, s.trafficRNG)
		if r.IsZero() {
			continue
		}
		// Ambushers get a distinct bracket name so "Marauder"/"Hierarch"
		// stay unambiguous for name-addressed scripts and overview filters.
		s.Names.Set(r, Name{Value: def.Name + " Ambusher"})
		raiders = append(raiders, r)
	}
	if len(raiders) == 0 {
		return
	}
	s.Warps.Remove(e)
	s.Intents.Set(e, Intent{Kind: IntentStop})

	now := s.time
	atk := &TrafficAttack{
		Raiders:   raiders,
		Started:   now,
		FleeAt:    now + trafficFleeDelaySec,
		RespondAt: -1,
	}
	if trafficPatrolWing(sec) > 0 {
		atk.RespondAt = now + float64(trafficResponseDelay(sec))*DT
	}
	tc.Attack = atk

	celID, celName := s.nearestLandmark(pos)
	s.emit(Event{Kind: EvTraffic, Phase: TrafficAttacked, Severity: SevWarning, Actor: e,
		System: s.systemID, Cel: celID, Item: tc.Item, Value: float64(len(raiders)),
		Text: "Convoy hauler under attack near " + celName})
}

// trafficEscaped records the escape outcome the moment the ambushed hauler
// gets its warp drive spinning again.
func (s *Sim) trafficEscaped(e ecs.Entity, tc *Traffic) {
	atk := tc.Attack
	atk.Outcome = TrafficOutcomeEscaped
	s.emit(Event{Kind: EvTraffic, Phase: TrafficEscaped, Severity: SevGood, Actor: e,
		System: s.systemID, Item: tc.Item,
		Text: "Convoy hauler warps clear of the ambush"})
	s.scheduleRaidersLeave(atk)
}

// trafficArrive resolves a completed leg: a station endpoint delivers the
// cargo into the system market (stock up, price one supply step down); any
// other endpoint despawns the hauler (it leaves the system through a gate
// or finishes its run at a belt).
func (s *Sim) trafficArrive(sys *content.SystemDef, e ecs.Entity, tc *Traffic) {
	dest := sysCelestial(sys, tc.Dest)
	if dest == nil {
		s.destroy(e)
		return
	}
	if dest.Station != nil {
		s.trafficDeliver(sys, e, tc, dest)
		return
	}
	s.destroy(e)
}

// trafficDeliver books the shipment: stock increases by exactly qty units
// and the item's price takes one supply step down, clamped into the
// security band.
func (s *Sim) trafficDeliver(sys *content.SystemDef, e ecs.Entity, tc *Traffic, dest *content.CelestialDef) {
	cargoValue := float64(tc.Qty) * s.marketBasePrice(s.systemID, tc.Item)
	idx := s.marketIndex(tc.Item)
	share := 1.0
	if idx >= 0 {
		// Turnover is the item's own shelf: its listed value at this system.
		share = trafficShare(cargoValue, float64(s.market[s.systemID][idx].Qty)*s.market[s.systemID][idx].Price)
	} else {
		// First shipment of an unlisted good opens the shelf at base price.
		if base := s.marketBasePrice(s.systemID, tc.Item); base > 0 {
			s.market[s.systemID] = append(s.market[s.systemID],
				MarketStock{Item: tc.Item, Price: base, Qty: 0})
			idx = len(s.market[s.systemID]) - 1
		}
	}
	if idx >= 0 {
		s.market[s.systemID][idx].Qty += tc.Qty // exact integer units
		s.marketPriceStep(s.systemID, idx, false, share, sys.Security, dest.Name)
	}

	vol := s.itemVolume(tc.Item) * float64(tc.Qty)
	volStr := num(vol)
	if math.Abs(vol-math.Round(vol)) < 1e-6 {
		volStr = itoaN(int(math.Round(vol)))
	}
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDelivered, Severity: SevGood, Actor: e,
		System: sys.ID, Cel: tc.Dest, Item: tc.Item, Value: float64(tc.Qty),
		Text: "Convoy delivery — " + volStr + " m³ " + s.itemName(tc.Item) +
			" received at " + dest.Name})
	s.destroy(e)
}

// trafficHaulerDestroyed resolves a lost convoy hauler (raider kill or
// player kill): the shipment turns into a lootable wreck (the caller adds
// the cargo), the destination market takes one risk-premium step up, and a
// running ambush resolves as destroyed with a scheduled raider break-off.
func (s *Sim) trafficHaulerDestroyed(e ecs.Entity, tc *Traffic) {
	sec, destName := "", ""
	if sys := s.System(); sys != nil {
		sec = sys.Security
		if d := sysCelestial(sys, tc.Dest); d != nil {
			destName = d.Name
		}
	}
	if idx := s.marketIndex(tc.Item); idx >= 0 && destName != "" {
		cargoValue := float64(tc.Qty) * s.marketBasePrice(s.systemID, tc.Item)
		share := trafficShare(cargoValue,
			float64(s.market[s.systemID][idx].Qty)*s.market[s.systemID][idx].Price)
		s.marketPriceStep(s.systemID, idx, true, share, sec, destName)
	}

	nearID, nearName := s.nearestLandmark(mustPos(s, e))
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDestroyed, Severity: SevDanger, Actor: e,
		System: s.systemID, Cel: nearID, Item: tc.Item, Value: float64(tc.Qty),
		Text: "Convoy hauler destroyed with its cargo near " + nearName})
	if atk := tc.Attack; atk != nil && atk.Outcome == "" {
		atk.Outcome = TrafficOutcomeDestroyed
		s.scheduleRaidersLeave(atk)
	}
}

// scheduleRaidersLeave books each live raider's break-off: after
// trafficRaidersLeaveSec the ship disengages and vanishes, whether or not
// the hauler (whose Traffic component owned the ambush) is still around.
func (s *Sim) scheduleRaidersLeave(atk *TrafficAttack) {
	for _, r := range atk.Raiders {
		if s.World.Alive(r) {
			s.trafficLeaves = append(s.trafficLeaves,
				TrafficLeave{Entity: r, At: s.time + trafficRaidersLeaveSec})
		}
	}
}

// ---------------------------------------------------------------------------
// Ambush combat and outcomes
// ---------------------------------------------------------------------------

// trafficDriveAttacks advances every live ambush: dispatches the patrol
// response when it is due, resolves a raid with no raiders left as saved
// (emitting EvTraffic phase "saved"), expires break-offs, and drives the
// raiders' weapons through the normal NPC activation policy (target =
// engaged patrol if one is close, else the hauler).
func (s *Sim) trafficDriveAttacks(sys *content.SystemDef, td *content.TrafficDef, sec string) {
	for _, e := range sortedEntities(s.Traffic) {
		tc, ok := s.Traffic.Get(e)
		if !ok || tc.Attack == nil {
			continue
		}
		atk := tc.Attack

		if !atk.Responded && atk.RespondAt >= 0 && s.time >= atk.RespondAt {
			atk.Responded = true
			s.trafficRespond(sys, e, td, sec)
		}
		if atk.Outcome == "" && !hasLiveRaiders(s, atk) {
			atk.Outcome = TrafficOutcomeSaved
			celID, celName := s.nearestLandmark(mustPos(s, e))
			s.emit(Event{Kind: EvTraffic, Phase: TrafficSaved, Severity: SevGood, Actor: e,
				System: s.systemID, Cel: celID, Item: tc.Item, Value: float64(tc.Qty),
				Text: "Convoy hauler saved near " + celName})
		}
		s.trafficRaidersAct(tc, atk, e)
	}
}

// trafficRaidersAct drives one ambush's raiders: they lock and activate on
// the hauler, switching everything (guns and ewar) onto a responding patrol
// that closes in — which is exactly what breaks the pin and saves the
// convoy. Raiding only happens while the raider's own AI state machine is
// idle: if it has aggroed the player, systemNpc owns it.
func (s *Sim) trafficRaidersAct(tc *Traffic, atk *TrafficAttack, hauler ecs.Entity) {
	for _, r := range atk.Raiders {
		if !s.World.Alive(r) {
			continue
		}
		ai, ok := s.AIs.Get(r)
		if !ok {
			continue
		}
		if ai.State != NpcIdle {
			// Engaged with the player: point its combat modules at the
			// player (targets bind at activation, so retarget explicitly).
			if ai.Target == s.player && (ai.State == NpcApproach || ai.State == NpcOrbit) {
				s.retargetWeapons(r, s.player)
			}
			continue
		}
		if atk.Outcome != "" {
			continue // incident resolved: weapons cold until break-off
		}
		target := s.nearbyRespondingPatrol(r)
		if target.IsZero() {
			if !s.World.Alive(hauler) {
				continue
			}
			target = hauler
		}
		s.retargetWeapons(r, target)
		s.npcEnsureLock(r, target)
		s.npcActivate(r, ai, target)
	}
}

// nearbyRespondingPatrol returns the closest responding patrol within
// engagement range of e, or zero.
func (s *Sim) nearbyRespondingPatrol(e ecs.Entity) ecs.Entity {
	ep, ok := s.posOf(e)
	if !ok {
		return 0
	}
	var best ecs.Entity
	bestD := math.MaxFloat64
	for _, p := range sortedEntities(s.Patrols) {
		st, ok := s.Patrols.Get(p)
		if !ok || st.State != PatrolRespond || !s.World.Alive(p) {
			continue
		}
		if d, ok := s.posOf(p); ok {
			if dist := d.Dist(ep); dist <= trafficPatrolEngageRange && dist < bestD {
				best, bestD = p, dist
			}
		}
	}
	return best
}

// hasLiveRaiders reports whether any raider of the ambush is still alive.
func hasLiveRaiders(s *Sim, atk *TrafficAttack) bool {
	for _, r := range atk.Raiders {
		if s.World.Alive(r) {
			return true
		}
	}
	return false
}

// trafficAttackOf returns the live ambush state of a hauler entity, or nil.
func (s *Sim) trafficAttackOf(e ecs.Entity) *TrafficAttack {
	if e.IsZero() || !s.World.Alive(e) {
		return nil
	}
	tc, ok := s.Traffic.Get(e)
	if !ok {
		return nil
	}
	return tc.Attack
}

// ---------------------------------------------------------------------------
// Patrol response
// ---------------------------------------------------------------------------

// trafficRespond dispatches the security response: a wing of floor(3σ)
// ships (fast and overwhelming in high-sec, a single late responder in
// low-sec, nothing in null), spawned at the anchor when the wing is under
// cap, otherwise reassigned from an idle patrol already on duty.
func (s *Sim) trafficRespond(sys *content.SystemDef, hauler ecs.Entity, td *content.TrafficDef, sec string) {
	if td == nil || td.Patrol == nil || len(td.Patrol.Hulls) == 0 {
		return
	}
	wing := trafficPatrolWing(sec)
	if wing <= 0 {
		return
	}
	dispatched := 0
	for i := 0; i < wing; i++ {
		var e ecs.Entity
		if s.Patrols.Len() >= trafficMaxPatrols {
			e = s.idlePatrol()
			if e.IsZero() {
				break
			}
		} else if e = s.spawnPatrolShip(sys, td.Patrol.Hulls[0]); e.IsZero() {
			break
		}
		if p, ok := s.Patrols.Get(e); ok {
			p.State = PatrolRespond
			p.Incident = hauler
			p.Next = 0
		}
		dispatched++
	}
	if dispatched == 0 {
		return
	}
	pos := mustPos(s, hauler)
	celID, celName := s.nearestLandmark(pos)
	s.emit(Event{Kind: EvTraffic, Phase: TrafficPatrolResponse, Severity: SevGood,
		System: sys.ID, Cel: celID, Value: float64(dispatched),
		Text: "Patrol wing responding to the ambush near " + celName})
}

// idlePatrol returns the first patrol sitting idle (canonical handle order),
// or zero.
func (s *Sim) idlePatrol() ecs.Entity {
	for _, e := range sortedEntities(s.Patrols) {
		if p, ok := s.Patrols.Get(e); ok && p.State == PatrolIdle && s.World.Alive(e) {
			return e
		}
	}
	return 0
}

// patrolResponding reports whether e is a security patrol working an
// incident (PatrolRespond). systemNpc skips its idle-patrol steering while
// true, so the responder holds station on the fight instead of circling
// home (patrolRespondStep owns its movement while it responds).
func (s *Sim) patrolResponding(e ecs.Entity) bool {
	p, ok := s.Patrols.Get(e)
	return ok && p.State == PatrolRespond
}

// trafficDrivePatrols runs responding/returning patrols: warp to the
// incident, lock and shoot raiders inside engagement range, then return to
// the anchor and resume the idle guard circle. Only idle AI states are
// driven here — an aggroed patrol belongs to systemNpc.
func (s *Sim) trafficDrivePatrols() {
	for _, e := range sortedEntities(s.Patrols) {
		p, ok := s.Patrols.Get(e)
		if !ok {
			continue
		}
		ai, hasAI := s.AIs.Get(e)
		if !hasAI {
			continue
		}
		if ai.State != NpcIdle {
			if ai.Target == s.player && (ai.State == NpcApproach || ai.State == NpcOrbit) {
				s.retargetWeapons(e, s.player)
			}
			continue
		}
		switch p.State {
		case PatrolRespond:
			s.patrolRespondStep(e, p, ai)
		case PatrolReturn:
			s.patrolReturnStep(e, p, ai)
		}
	}
}

// patrolRespondStep advances one responding patrol toward and into the
// fight, or sends it home when the raid is over.
func (s *Sim) patrolRespondStep(e ecs.Entity, p *Patrol, ai *NpcAI) {
	atk := s.trafficAttackOf(p.Incident)
	if atk == nil || !hasLiveRaiders(s, atk) {
		p.State = PatrolReturn
		p.Incident = 0
		p.Next = 0
		return
	}
	if s.Warps.Has(e) {
		return // warp state machine owns movement
	}
	// Hold station on the incident: systemNpc no longer runs the idle-patrol
	// loop for a responding patrol, and a patrol intent left over from
	// before the dispatch must not steer the ship back to its Home circle
	// mid-fight (it would drift out of engagement range of the raiders).
	s.Intents.Set(e, Intent{Kind: IntentStop})
	// Closest live raider.
	ep, ok := s.posOf(e)
	if !ok {
		return
	}
	var target ecs.Entity
	bestD := math.MaxFloat64
	for _, r := range atk.Raiders {
		if !s.World.Alive(r) {
			continue
		}
		if rp, ok := s.posOf(r); ok {
			if d := rp.Dist(ep); d < bestD {
				target, bestD = r, d
			}
		}
	}
	if target.IsZero() {
		p.State = PatrolReturn
		return
	}
	if bestD > trafficPatrolEngageRange {
		if tp, ok := s.posOf(target); ok && s.time >= p.Next && s.warpToPoint(e, tp) {
			p.Next = s.time + trafficWarpRetrySec
		}
		return
	}
	s.retargetWeapons(e, target)
	s.npcEnsureLock(e, target)
	s.npcActivate(e, ai, target)
}

// patrolReturnStep flies a patrol back to its anchor and resumes the idle
// guard circle on arrival.
func (s *Sim) patrolReturnStep(e ecs.Entity, p *Patrol, ai *NpcAI) {
	if s.Warps.Has(e) {
		return
	}
	ep, ok := s.posOf(e)
	if !ok {
		return
	}
	if ep.Dist(ai.Home) <= warpLandRange {
		p.State = PatrolIdle
		return
	}
	if s.time >= p.Next && s.warpToPoint(e, ai.Home) {
		p.Next = s.time + trafficWarpRetrySec
	}
}

// ---------------------------------------------------------------------------
// Market drift
// ---------------------------------------------------------------------------

// marketDecay halves every deviating price's distance to base once per
// marketDecayTicks (the pinned halving schedule). Silent by design: only
// traffic-driven moves emit events.
func (s *Sim) marketDecay() {
	if (s.ticks+1)%marketDecayTicks != 0 {
		return
	}
	ids := make([]content.SystemID, 0, len(s.market))
	for id := range s.market {
		ids = append(ids, id)
	}
	sortSystemIDs(ids)
	for _, id := range ids {
		stock := s.market[id]
		for i := range stock {
			base := s.marketBasePrice(id, stock[i].Item)
			if base <= 0 {
				continue
			}
			stock[i].Price = base + (stock[i].Price-base)*0.5
		}
	}
}

// marketBasePrice returns the un-drifted reference price of an item at a
// system: the authored market order's price, else the item's base price
// (modules fall back to their authored price).
func (s *Sim) marketBasePrice(sys content.SystemID, item content.ItemID) float64 {
	if sysDef, ok := s.Content.System(sys); ok {
		for _, o := range sysDef.Market {
			if o.Item == item {
				return o.Price
			}
		}
	}
	if def, ok := s.Content.Item(item); ok {
		return def.BasePrice
	}
	if def, ok := s.Content.Module(content.ModuleID(item)); ok {
		return def.Price
	}
	return 0
}

// trafficShare is the cargo's value share of the station's turnover for
// that commodity (its shelf value), capped at 1. A shelf that does not
// exist yet counts as fully created by the shipment.
func trafficShare(cargoValue, shelfValue float64) float64 {
	if shelfValue <= 0 {
		return 1
	}
	v := cargoValue / shelfValue
	if v > 1 {
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}

// marketPriceStep moves one order's price by one drift step:
//
//	price' = clamp(base·(1±band), price·(1±k))
//	k      = share · band / 2.5, doubled for a lost shipment (risk premium)
//
// A step that would leave the band clamps silently (no event, no change).
func (s *Sim) marketPriceStep(sys content.SystemID, idx int, up bool, share float64, security, where string) {
	stock := s.market[sys]
	if idx < 0 || idx >= len(stock) {
		return
	}
	item := stock[idx].Item
	base := s.marketBasePrice(sys, item)
	if base <= 0 {
		return
	}
	band := trafficBand(security)
	k := share * band / trafficDriftDiv
	if up {
		k *= 2
	}
	if k <= 0 {
		return
	}
	p := stock[idx].Price
	if up {
		p *= 1 + k
	} else {
		p *= 1 - k
	}
	p = clamp(p, base*(1-band), base*(1+band))
	if p == stock[idx].Price {
		return
	}
	stock[idx].Price = p
	verb, direction := "Surplus", "falls to"
	if up {
		verb, direction = "Risk premium", "rises to"
	}
	s.emit(Event{Kind: EvMarket, Severity: SevInfo, System: sys, Item: item, Value: p,
		Text: verb + " — " + s.itemName(item) + " price " + direction + " " +
			euro(p) + " at " + where})
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// warpToPoint engages the warp drive toward a bare point (NPC traffic and
// patrols warp through the same align/tunnel/land profile as the player).
// The warp system cancels the attempt if the ship is scrambled; callers
// retry on their cooldown.
func (s *Sim) warpToPoint(e ecs.Entity, to core.Vec2) bool {
	if s.Warps.Has(e) {
		return false
	}
	s.Warps.Set(e, WarpDrive{Phase: "align", To: to})
	s.Intents.Set(e, Intent{Kind: IntentStop})
	return true
}

// retargetWeapons re-points active weapon/ewar modules when the desired
// target changed: module targets bind at activation time, so the cycle must
// be dropped and re-armed (tank/cap/prop modules self-target and are left
// alone).
func (s *Sim) retargetWeapons(e ecs.Entity, target ecs.Entity) {
	fit, ok := s.Fittings.Get(e)
	if !ok {
		return
	}
	for _, class := range slotClasses {
		defs, rts := fit.Defs(class), fit.Runtimes(class)
		for i := range rts {
			if !rts[i].Active {
				continue
			}
			def, ok := s.Content.Module(defs[i])
			if !ok {
				continue
			}
			switch def.Kind {
			case content.KindWeapon, content.KindStasisWeb, content.KindWarpScram:
			default:
				continue
			}
			slot := SlotRef{Class: class, Index: i}
			if cur, ok := s.modTarget(e, slot); ok && cur == target {
				continue
			}
			s.deactivate(e, slot, "", SevInfo)
		}
	}
}

// isTrafficRaider reports whether e is a raider in any live ambush.
func (s *Sim) isTrafficRaider(e ecs.Entity) bool {
	found := false
	s.Traffic.Each(func(_ ecs.Entity, tc *Traffic) {
		if found || tc.Attack == nil {
			return
		}
		for _, r := range tc.Attack.Raiders {
			if r == e {
				found = true
				return
			}
		}
	})
	return found
}

// sysCelestial returns the celestial definition with the given ID inside sys.
func sysCelestial(sys *content.SystemDef, id content.CelID) *content.CelestialDef {
	for i := range sys.Celestials {
		if sys.Celestials[i].ID == id {
			return &sys.Celestials[i]
		}
	}
	return nil
}

// nearestLandmark returns the ID and name of the landmark celestial closest
// to pos ("deep space" when the grid has none).
func (s *Sim) nearestLandmark(pos core.Vec2) (content.CelID, string) {
	e := s.nearestCelestial(pos)
	if e.IsZero() {
		return "", "deep space"
	}
	if c, ok := s.Celestials.Get(e); ok {
		return c.Cel, s.nameOf(e)
	}
	return "", s.nameOf(e)
}

// mustPos returns e's position (zero when absent — only used for events on
// entities that were just alive).
func mustPos(s *Sim, e ecs.Entity) core.Vec2 {
	if p, ok := s.posOf(e); ok {
		return p
	}
	return core.Vec2{}
}

// sortSystemIDs orders system IDs ascending (deterministic map iteration).
func sortSystemIDs(ids []content.SystemID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
