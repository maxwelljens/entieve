package sim

import (
	"math"
	"sort"

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
//	patrol cadence         authored / σ (σ = 0: no patrols)
//	patrol response        delay 1800x(1-σ) ticks, wing floor(3σ)
//
// Prices are NOT in this table anymore: the per-station order books are the
// only price former (see system_economy.go). Deliveries post supply on the
// destination's book; a lost shipment is an event-only supply-risk signal.
//
// Cargo provenance — route cargo is SOURCED at spawn, never conjured:
//   - station origin  → marketWithdraw from that station's own asks (the
//     weighted price paid becomes the shipment's cost basis);
//   - belt origin     → the volume is drawn from the persistent belt state
//     (dry belt → no shipment this cycle);
//   - gate origin     → the world boundary: shipments arriving from outside
//     the simulated region spawn undeighted and post as supply only at the
//     destination. This is the economy's one sanctioned import — everything
//     inside the ring is conserved.
//
// An origin without the stock spawns nothing this cycle (an informational
// event, never a conjured unit). The debited goods ride the hauler's hold;
// a destroyed hauler takes them to the wreck (lootable) and no reversal
// ever touches the origin book. Cross-system hauls park the shipment as a
// journey (Save.TrafficJourneys) when they cross a gate or when the grid
// rebuilds under them, and continue when the destination system loads.
// The parked ledger is a closed queue behind three caps: resume is
// staggered and shares trafficMaxLive with fresh spawns, a gate hop into a
// system already at its parked-journey ceiling holds upstream instead
// (trafficMaxParkedJourneys), and an ambush that would break
// trafficMaxRaiders does not spawn — none of them ever drops, duplicates
// or creates cargo.
//
// Haul-wish convoys (the consumption seam for agent.go's HaulWish) are the
// second spawn path: on a slow stateless poll the system walks the agents'
// canonical wish list and launches the first wish this grid can serve.
// Their provenance is the AGENT's own stock (its resting asks at the
// origin first, then free stock — debited at spawn like every other
// shipment), they share trafficMaxLive and the journey ledger with the
// authored lanes, and their Traffic.Owner tag rides every park/resume so
// delivery lists the cargo at the agent's OWN ask and settles the fee
// through FulfillHaul. Authored lanes schedule first; wishes only fill
// what is left of the cap.
// ---------------------------------------------------------------------------

const (
	// trafficStreamID keys the forked living-universe RNG stream (master
	// state XOR splitMix64(id)): convoys and patrols never consume
	// master-stream rolls, so existing seed-pinned outcomes stay put.
	trafficStreamID uint64 = 0x7472_6166_6669_6301

	// trafficMaxLive caps live convoy haulers per system; trafficMaxPatrols
	// caps patrol ships. Spawn attempts beyond the cap are skipped (the
	// schedule itself is stateless, so skips never shift future attempts).
	// The resume path shares trafficMaxLive with the fresh schedules, so a
	// deep parked ledger can never unload as one fleet.
	trafficMaxLive    = 6
	trafficMaxPatrols = 16

	// trafficMaxRaiders caps CONCURRENT live traffic raiders per grid —
	// static spawnGroup NPCs and patrols are not raiders and never count.
	// trafficMaxParkedJourneys is the per-system gate queue: trafficHop
	// never parks a journey past it (the shipment holds upstream instead).
	// trafficResumeInterval staggers resumed convoy departures so a backlog
	// spreads across ticks instead of aligning and warping in one tick.
	trafficMaxRaiders        = 6
	trafficMaxParkedJourneys = 3 * trafficMaxLive // 18 parked journeys per system
	trafficResumeInterval    = 30                 // ticks between resumed-departure opportunities
	trafficAmbushRaidersMax  = 3                  // one ambush drops in 1..3 raiders
	trafficMinRaiders        = 1

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

	// patrolStationRMin/patrolStationRMax bound the station ring a responding
	// wing holds around its incident (m): each responder owns a point on it,
	// so a ten-ship wing reads as a formation instead of one stacked hull.
	// Still far inside trafficPatrolEngageRange.
	patrolStationRMin = 1800.0
	patrolStationRMax = 3000.0

	// trafficWishInterval is the wish-consumption poll cadence (ticks) —
	// stateless like every traffic schedule (trafficDue), so a skipped
	// attempt never reschedules anything. One shipment per poll: convoys
	// trickle in where the spread justifies them instead of grabbing the
	// whole live cap at once.
	trafficWishInterval = 60

	// trafficHaulFee pins what a haul-wish shipment pays its hauler:
	//
	//	cost = trafficHaulFee × transit legs × cargo volume (m³)
	//
	// i.e. 2.5 € per m³ per leg (250 € moves 100 m³ one leg). Freight is
	// volume-priced, so bulky low-value cargo only moves when its spread
	// really pays, and zero-volume cargo (modules, hulls) rides free. The
	// fee is a pure sink: FulfillHaul debits the agent's free capital and
	// credits nothing.
	trafficHaulFee = 2.5

	// --- The beat: paced, jittered, deterministic commencement. Nothing
	// commences the tick it becomes possible — spawn, resume, gate arrival
	// and station arrival all hold a window first (CONTEXT.md: departure
	// beat / gate cycle / turnaround; docs/SIM.md). Every window is in
	// ticks and jittered per event from trafficPhase (a pure hash — no
	// entropy, no persisted timers: the running deadline rides the
	// Traffic.Next / Traffic.Phase fields that are already in the save
	// schema). Delays move only WHEN things happen; cargo and money never
	// move early, twice or not at all.
	//
	// departure beat         spawn/resume → leg warp       120..480 (2..8 s)
	// gate cycle: dwell      at the gate before the hop    120..300 (2..5 s)
	// gate cycle: jump       hop-out → materialize         60..180  (1..3 s)
	// turnaround: approach   landing → station contact     120..240 (2..4 s)
	// turnaround: dock       contact → cargo action        120..300 (2..5 s)
	// turnaround: berth      cargo action → undock         180..600 (3..10 s)
	// turnaround: undock     undock → despawn (beat)       120..480 (2..8 s)
	//
	// (dock+berth is the docked dwell: 300..900 ticks, 5..15 s, with the
	// cargo/market action firing partway through.)
	trafficDepartBeatMin, trafficDepartBeatMax = 120, 480
	trafficGateDwellMin, trafficGateDwellMax   = 120, 300
	trafficJumpCycleMin, trafficJumpCycleMax   = 60, 180
	trafficApproachMin, trafficApproachMax     = 120, 240
	trafficDockMin, trafficDockMax             = 120, 300
	trafficBerthMin, trafficBerthMax           = 180, 600
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

// trafficPatrolWing is the response wing size: a full flight of ten in
// high-sec (the answer is overwhelming), one late cutter in low, none in
// null.
func trafficPatrolWing(security string) int {
	switch security {
	case "high":
		return 10
	case "null":
		return 0
	default:
		return 1
	}
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

// trafficBeat is a jittered delay in [min, max] ticks derived from the
// same pure hash the schedules use — stateless jitter for the paced
// commencement (departure beat, gate cycle, turnaround): no entropy is
// drawn, no timer is persisted, and the same key always yields the same
// delay in twin runs and after a restore.
func trafficBeat(key string, min, max int64) int64 {
	if max < min {
		max = min
	}
	return min + int64(trafficPhase(key)%uint64(max-min+1))
}

// beatKey is a jitter key unique to one live hauler event: the entity
// handle (reproduced exactly by twin runs and by save/restore) plus the
// event kind, so identical shipments never share one beat.
func beatKey(e ecs.Entity, kind string) string {
	return "beat|" + kind + "|" + itoaN(int(uint64(e)))
}

// ---------------------------------------------------------------------------
// The system
// ---------------------------------------------------------------------------

// systemTraffic advances the living-universe layer: parked journeys resume
// in this grid, convoy haulers spawn on their security-scaled schedules
// (cargo sourced at spawn — see the provenance contract in this file's
// header) and fly their legs through the warp profile; each tunnel entry
// rolls the per-leg ambush chance; live ambushes drive raiders (real
// lock/activate/turret/damage pipeline), dispatch the patrol response, and
// resolve into destroyed / escaped / saved outcomes; a delivered shipment
// posts supply on the destination station's book (the order books form
// prices — see system_economy.go), while a cross-system hauler that
// reaches a gate parks its shipment for the far system to continue.
//
// Runs after every other system: warp landings, deaths and NPC state are
// already settled for the tick, and any intents set here fly next tick's
// movement pass.
func (s *Sim) systemTraffic() {
	sys := s.System()
	if sys == nil {
		return
	}
	// Shipments waiting in this grid (gate hops, reloads) fly on before
	// fresh spawns: they are owed cargo, and they count against the live cap.
	s.trafficResumeJourneys(sys)
	var td *content.TrafficDef
	if t, ok := s.Content.TrafficFor(sys.ID); ok {
		td = t
	}
	sec := s.EffectiveSecurity(sys.ID)
	if td != nil {
		s.trafficSpawnHaulers(sys, td, sec)
		s.trafficSpawnPatrol(sys, td, sec)
	}
	// Wishes fill what is left of the live cap (authored lanes schedule first).
	s.trafficSpawnWishHaulers(sys)
	s.trafficDriveHaulers(sys, td, sec)
	s.trafficDriveAttacks(sys, td, sec)
	s.trafficDrivePatrols(sys, td, sec)
	s.trafficExpireLeaves()
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
// due and the live cap allows. A due attempt whose origin carries no stock
// fires nothing (trafficSource emits the event and draws no entropy).
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
// celestial and kicks off its first leg. The cargo is sourced out of the
// world BEFORE the ship exists (trafficSource): an origin without the
// stock spawns nothing this cycle — route cargo is never conjured. The
// first leg ends at the route's destination (same system) or at the
// next-hop gate toward it (cross-system route).
func (s *Sim) spawnTrafficHauler(sys *content.SystemDef, td *content.TrafficDef, r content.RouteDef) {
	hull, ok := s.Content.Ship(r.Hull)
	if !ok {
		return
	}
	from := sysCelestial(sys, r.From)
	if from == nil {
		return
	}
	destSys := r.ToSystem
	if destSys == "" {
		destSys = sys.ID
	}
	destDef, ok := s.Content.System(destSys)
	if !ok {
		return
	}
	to := sysCelestial(destDef, r.To)
	if to == nil {
		return
	}
	target := r.To
	if destSys != sys.ID {
		if target = s.trafficNextHop(sys.ID, destSys); target == "" {
			return
		}
	}
	basis, ok := s.trafficSource(sys, from, r.Cargo)
	if !ok {
		return
	}

	spread := from.Radius
	if spread < 1000 {
		spread = 1000
	}
	angle := s.trafficRNG.Range(0, 2*math.Pi)
	rad := s.trafficRNG.Range(0, spread)
	pos := from.Position.Add(core.Vec2{X: math.Cos(angle) * rad, Y: math.Sin(angle) * rad})

	e := s.createHauler(hull, pos, angle, Traffic{
		Origin: r.From, Dest: target, DestSys: destSys, DestCel: r.To,
		Item: r.Cargo.Item, Qty: r.Cargo.Qty, Basis: basis, Phase: TrafficTransit,
	})
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDeparted, Severity: SevInfo, Actor: e,
		System: sys.ID, Cel: r.From, Item: r.Cargo.Item, Value: float64(r.Cargo.Qty),
		Text: "Convoy hauler departs " + from.Name + " bound for " + to.Name})
}

// createHauler builds a convoy hauler entity: ship, hold carrying exactly
// the shipment, targeting, and the Traffic component (spawn state or a
// resumed journey's continuation state).
func (s *Sim) createHauler(hull *content.ShipDef, pos core.Vec2, facing float64, tc Traffic) ecs.Entity {
	e := s.create()
	s.Names.Set(e, Name{Value: hull.Name})
	s.Transforms.Set(e, Transform{Pos: pos, Facing: facing})
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
		Stacks:      []content.CargoStack{{Item: tc.Item, Qty: tc.Qty}},
		Capacity:    hull.Cargo,
		OreCapacity: hull.OreHold,
	})
	s.Targetings.Set(e, Targeting{
		MaxTargets: hull.MaxTargets, MaxRange: hull.MaxTargetRange, ScanRes: hull.ScanResolution,
	})
	// Departure beat: every hauler — fresh spawn, wish convoy, resumed
	// journey — holds before its first warp commencement (the drive loop's
	// s.time >= tc.Next gate below). A carried journey deadline is
	// superseded here; it gated the resume itself.
	tc.Next = s.time + float64(trafficBeat(beatKey(e, "depart"),
		trafficDepartBeatMin, trafficDepartBeatMax))*DT
	s.Traffic.Set(e, tc)
	s.recomputeFit(e)
	s.healToFull(e)
	return e
}

// trafficSource draws a route's cargo out of the world at SPAWN time and
// returns its cost basis (see the provenance contract in the header). An
// origin without the stock spawns nothing this cycle.
func (s *Sim) trafficSource(sys *content.SystemDef, from *content.CelestialDef, cargo content.RouteCargo) (float64, bool) {
	switch {
	case from.Station != nil:
		// The availability check and the withdrawal run back-to-back in one
		// tick — nothing mutates the book between them, so the take is
		// always the full shipment (no partial draw to leak).
		if s.marketAskQty(from.ID, cargo.Item) < cargo.Qty {
			s.noStockToShip(sys, from, cargo)
			return 0, false
		}
		taken, paid := s.marketWithdraw(from.ID, cargo.Item, cargo.Qty)
		if taken < cargo.Qty { // unreachable: see above
			s.noStockToShip(sys, from, cargo)
			return 0, false
		}
		return paid / float64(taken), true
	case from.Belt != nil:
		need := s.itemVolume(cargo.Item) * float64(cargo.Qty)
		if !s.beltOreAvailable(sys.ID, need) {
			s.noStockToShip(sys, from, cargo)
			return 0, false
		}
		if !s.drawBeltOre(sys.ID, need) { // dry run passed: cannot fail
			s.noStockToShip(sys, from, cargo)
			return 0, false
		}
		return s.marketBasePrice(cargo.Item), true
	case from.Gate != nil:
		// Import boundary: goods arriving from outside the simulated region.
		// Nothing is debited anywhere — the gate is where the ring's
		// conservation stops, and the shipment posts as supply at the
		// destination like every other convoy delivery.
		return s.marketBasePrice(cargo.Item), true
	default:
		// Stars/planets carry no stock (route validation rejects them as
		// endpoints anyway): never conjure.
		s.noStockToShip(sys, from, cargo)
		return 0, false
	}
}

// noStockToShip announces a spawn attempt skipped for want of stock: the
// route's origin simply does not carry the cargo right now.
func (s *Sim) noStockToShip(sys *content.SystemDef, from *content.CelestialDef, cargo content.RouteCargo) {
	s.emit(Event{Kind: EvLog, Severity: SevInfo, System: sys.ID, Cel: from.ID,
		Item: cargo.Item, Value: float64(cargo.Qty),
		Text: "no " + s.itemName(cargo.Item) + " to ship from " + from.Name})
}

// marketAskQty sums a station's resting asks for an item — the dry run
// before the spawn-time provenance withdrawal (marketWithdraw takes asks
// in price-time order, so a covered check always yields a full take).
func (s *Sim) marketAskQty(station StationID, item content.ItemID) int {
	book := s.market[station]
	if book == nil {
		return 0
	}
	n := 0
	for _, o := range book.Orders {
		if o.Item == item && o.Side == OrderSell {
			n += o.Qty
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Haul-wish convoys (the wish-consumption seam)
// ---------------------------------------------------------------------------

// trafficSpawnWishHaulers walks every agent's pending haul wishes in
// canonical order (agents ascending, each list in (From, To, Item) order)
// and launches the first one this grid can actually serve — one shipment
// per poll, the cap shared with the authored lanes. Attempts that fail a
// check leave the wish pending and draw no entropy, exactly like a skipped
// authored schedule (the poll itself is stateless: trafficDue).
func (s *Sim) trafficSpawnWishHaulers(sys *content.SystemDef) {
	if s.Traffic.Len() >= trafficMaxLive {
		return
	}
	if !trafficDue(s.ticks, string(sys.ID)+"|wish", trafficWishInterval) {
		return
	}
	for _, w := range s.HaulWishes() {
		if s.trafficShipWish(sys, w) {
			return
		}
	}
}

// trafficShipWish tries to launch one wish as a real convoy. Re-validates
// the opportunity against live state before anything mutates: the haul
// starts on this grid (its origin station is here), the gate path exists,
// the net spread still covers the pinned haul cost and stays inside the
// wish's MaxCost, and the agent can fund the fee. Only then are the goods
// sourced out of the agent's stock and the hauler built — the same
// scatter draws, in the same order, as an authored spawn (2 draws).
func (s *Sim) trafficShipWish(sys *content.SystemDef, w HaulWish) bool {
	a := s.agentByOwner(w.Owner)
	if a == nil || w.Qty <= 0 {
		return false
	}
	if _, shipping := a.inFlightFor(w.Item, w.Qty, w.To); shipping {
		return false // one shipment per wish
	}
	fromSys, ok := s.stationSystem(w.From)
	if !ok || fromSys != sys.ID {
		return false // origins only spawn where their station's grid is loaded
	}
	from := sysCelestial(sys, w.From)
	toSys, ok := s.stationSystem(w.To)
	if !ok {
		return false
	}
	destDef, ok := s.Content.System(toSys)
	if !ok {
		return false
	}
	to := sysCelestial(destDef, w.To)
	if to == nil || to.Station == nil {
		return false
	}
	target := w.To
	if toSys != sys.ID {
		if target = s.trafficNextHop(sys.ID, toSys); target == "" {
			return false // no gate path: the wish can never ship
		}
	}
	legs := s.trafficWishLegs(w.From, w.To)
	if legs == 0 {
		return false
	}
	hull := s.wishHullFor(s.itemVolume(w.Item) * float64(w.Qty))
	if hull == nil {
		return false // no hold in the content takes the shipment
	}
	cost := s.trafficWishCost(w)
	// Re-validate the spread against the books (before the sourcing mutates
	// them): the net spread must still cover the haul, and the haul must
	// still fit the budget the wish was authored with.
	lo := s.MarketPrice(w.From, w.Item)
	hi := s.MarketPrice(w.To, w.Item)
	net := hi - lo - (MarketBrokerFee+MarketSalesTax)*lo
	if cost > w.MaxCost || net < cost || a.freeCapital() < cost {
		return false
	}
	// Source the goods last: the agent's own asks at the origin first,
	// then free stock (never a partial, never conjured).
	if !s.haulSourceGoods(a, w) {
		return false
	}

	spread := from.Radius
	if spread < 1000 {
		spread = 1000
	}
	angle := s.trafficRNG.Range(0, 2*math.Pi)
	rad := s.trafficRNG.Range(0, spread)
	pos := from.Position.Add(core.Vec2{X: math.Cos(angle) * rad, Y: math.Sin(angle) * rad})

	e := s.createHauler(hull, pos, angle, Traffic{
		Origin: w.From, Dest: target, DestSys: toSys, DestCel: w.To,
		Item: w.Item, Qty: w.Qty, Basis: lo, Owner: w.Owner,
		Phase: TrafficTransit,
	})
	a.beginShipment(w)
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDeparted, Severity: SevInfo, Actor: e,
		System: sys.ID, Cel: w.From, Item: w.Item, Value: float64(w.Qty),
		Text: "Convoy hauler departs " + from.Name + " bound for " + to.Name})
	return true
}

// trafficWishLegs counts the transit legs of a station-to-station haul:
// one warp per system on the gate path (origin → first gate, gate → gate,
// …, last gate → destination). 0 when either end is no station or the two
// systems are unconnected — a haul that can never run.
func (s *Sim) trafficWishLegs(from, to StationID) int {
	fromSys, ok := s.stationSystem(from)
	if !ok {
		return 0
	}
	toSys, ok := s.stationSystem(to)
	if !ok {
		return 0
	}
	if fromSys == toSys {
		return 1
	}
	legs := 1
	cur := fromSys
	for i := 0; i < len(s.Content.Systems); i++ { // bounded: each step enters a new system
		gate := s.trafficNextHop(cur, toSys)
		if gate == "" {
			return 0
		}
		def, ok := s.Content.System(cur)
		if !ok {
			return 0
		}
		cel := sysCelestial(def, gate)
		if cel == nil || cel.Gate == nil {
			return 0
		}
		cur = cel.Gate.ToSystem
		legs++
		if cur == toSys {
			return legs
		}
	}
	return 0
}

// trafficWishCost is the fee a wish shipment owes on delivery (the pin in
// trafficHaulFee): transit legs × cargo volume × rate. Pure function of
// content and the wish, so spawn and delivery compute the same number.
func (s *Sim) trafficWishCost(w HaulWish) float64 {
	legs := s.trafficWishLegs(w.From, w.To)
	return trafficHaulFee * float64(legs) * s.itemVolume(w.Item) * float64(w.Qty)
}

// wishHullFor picks the hull a wish convoy flies: the first ship id in
// canonical order whose hold takes the shipment (content-derived, no RNG —
// every run picks the same hull), nil when no hull fits.
func (s *Sim) wishHullFor(vol float64) *content.ShipDef {
	ids := make([]content.ShipID, 0, len(s.Content.Ships))
	for id := range s.Content.Ships {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if d, ok := s.Content.Ship(id); ok && d.Cargo >= vol {
			return d
		}
	}
	return nil
}

// trafficLoseWish drops the wish a lost/stranded shipment was carrying:
// the goods were debited at spawn and never arrived, so the agent is NOT
// paid (no FulfillHaul) and the wish leaves the list — the next behavior
// pass rebuilds from live spreads anyway. Authored shipments (Owner "")
// are untouched.
func (s *Sim) trafficLoseWish(tc *Traffic) {
	if tc.Owner == "" {
		return
	}
	dest := tc.DestCel
	if dest == "" { // pre-cross-system Traffic: the leg target is the destination
		dest = tc.Dest
	}
	a := s.agentByOwner(tc.Owner)
	if a == nil {
		return
	}
	if w, ok := a.inFlightFor(tc.Item, tc.Qty, StationID(dest)); ok {
		a.dropInFlight(w)
		a.dropWish(w)
	}
}

// trafficLoseJourney is trafficLoseWish for a parked shipment that can no
// longer continue (see trafficJourneyLost).
func (s *Sim) trafficLoseJourney(j TrafficJourney) {
	if j.Owner == "" {
		return
	}
	tc := Traffic{Owner: j.Owner, Item: j.Item, Qty: j.Qty,
		Dest: j.DestCel, DestCel: j.DestCel}
	s.trafficLoseWish(&tc)
}

// trafficSettleWish closes a delivered wish shipment: the pinned haul fee
// is charged through FulfillHaul (free capital, a sink) and the wish drops
// from the agent's list; the in-flight commitment is released either way,
// so a shipment can never settle twice.
func (s *Sim) trafficSettleWish(a *Agent, tc *Traffic, dest StationID) {
	w, ok := a.inFlightFor(tc.Item, tc.Qty, dest)
	if !ok {
		return
	}
	s.FulfillHaul(w, s.trafficWishCost(w))
	a.dropInFlight(w)
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
// sit out the gate cycle / turnaround at its endpoint (trafficArrive — no
// commencement on the tick of arrival), hop the gate (cross-system leg),
// deliver, or despawn at a same-system gate/belt on arrival, and issue the
// leg warp only once the departure beat in Traffic.Next has run.
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
			s.trafficLoseWish(tc) // destination vanished (shouldn't happen): drop the leg
			s.destroy(e)
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

// startAmbush drops 1-3 raiders of the system's local rat class into the
// fight mid-leg, interrupts the hauler's warp, and schedules the patrol
// response.
//
// Raider insertion is a warp-in from off-grid, not a pop-in: they spawn on
// the hauler's warp path ahead of it (WarpMinRange out — "the tunnel",
// entered off-grid) and run the player's own align/tunnel/decel process
// onto the fight grid, landing on the scatter slots their spawn rolls would
// have given them around the hauler. Same scatter draws, same count as
// dropping them on the hauler — the insertion only moves where the rolls
// are measured from. A fresh spawn has zero velocity, so the align gate
// opens at 0.75 × Agility × 10 s (systemMovement's align law).
func (s *Sim) startAmbush(e ecs.Entity, tc *Traffic, td *content.TrafficDef, sec string) {
	def, ok := s.Content.Npc(td.Raider)
	if !ok {
		return
	}
	pos, ok := s.posOf(e)
	if !ok {
		return
	}
	// The insertion point sits off-grid on the hauler's side of the fight:
	// along its warp vector when it is in warp, else along the leg toward
	// its destination. Pure geometry — no RNG draws.
	var dir core.Vec2
	if wd, ok := s.Warps.Get(e); ok {
		dir = wd.To.Sub(pos)
	} else if dp, ok := s.posOf(s.celEntity(tc.Dest)); ok {
		dir = dp.Sub(pos)
	}
	if dir.IsZero() {
		dir = core.Vec2{X: 1}
	}
	base := pos.Add(dir.Norm().Scale(WarpMinRange))
	n := trafficMinRaiders + s.trafficRNG.Intn(trafficAmbushRaidersMax-trafficMinRaiders+1)
	// Raider ceiling (per grid): the count draw above is consumed on every
	// successful ambush roll in a FIXED order — the ceiling never re-orders
	// or skips draws before it, so the traffic stream diverges from an
	// uncapped run only when the check below actually refuses a spawn.
	// Refusing the whole ambush (rather than clamping n) keeps twin runs
	// and post-restore continuations bit-identical. Static spawnGroup NPCs
	// and patrols are not raiders and never count toward the ceiling.
	if s.trafficLiveRaiders()+n > trafficMaxRaiders {
		return
	}
	raiders := make([]ecs.Entity, 0, n)
	for i := 0; i < n; i++ {
		r := s.spawnNpcStream(td.Raider, base, trafficAmbushSpread, s.trafficRNG)
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
	for _, r := range raiders {
		rp, ok := s.Transforms.Get(r)
		if !ok {
			continue
		}
		// Home is the fight grid (what a hauler-side spawn used), never the
		// off-grid insertion point: patrol circle, aggro and leash all keep
		// measuring from the ambush site.
		if ai, ok := s.AIs.Get(r); ok {
			ai.Home = pos
		}
		// Land on this raider's own scatter slot around the fight: the
		// roll produced (base + offset); the fight grid carries the offset.
		s.warpToPoint(r, pos.Add(rp.Pos.Sub(base)))
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

// trafficArrive resolves a hauler standing at its leg's endpoint — on the
// beat: an arrival never commences the next transition in the same tick.
// A gate endpoint runs the gate cycle (dwell → hop), a station endpoint
// runs the turnaround (approach → docked dwell with the cargo action
// partway through → undock + departure beat), any other endpoint despawns
// as before. First contact (Phase == TrafficTransit) arms the stage's
// deadline in Traffic.Next; later ticks wait it out — both fields are
// already in the save schema, so a snapshot taken mid-stage resumes that
// stage to the same deadline.
func (s *Sim) trafficArrive(sys *content.SystemDef, e ecs.Entity, tc *Traffic) {
	dest := sysCelestial(sys, tc.Dest)
	if dest == nil {
		s.trafficLoseWish(tc)
		s.destroy(e)
		return
	}
	if tc.DestSys != "" && tc.DestSys != sys.ID {
		s.trafficGateCycle(sys, e, tc, dest)
		return
	}
	if dest.Station != nil {
		s.trafficTurnaround(sys, e, tc, dest)
		return
	}
	s.trafficLoseWish(tc) // a wish never routes to a belt/gate end (To is a station)
	s.destroy(e)
}

// trafficGateCycle is the gate half of the beat: the hauler dwells at the
// gate it reached (jittered statelessly) before trafficHop runs, so a
// cross-system shipment never teleports onward on the tick of arrival.
func (s *Sim) trafficGateCycle(sys *content.SystemDef, e ecs.Entity, tc *Traffic, gate *content.CelestialDef) {
	if tc.Phase != TrafficGate { // first contact: arm the dwell
		tc.Phase = TrafficGate
		tc.Next = s.time + float64(trafficBeat(beatKey(e, "gate"),
			trafficGateDwellMin, trafficGateDwellMax))*DT
		return
	}
	if s.time < tc.Next {
		return
	}
	s.trafficHop(sys, e, tc, gate)
}

// trafficTurnaround is the station half of the beat: approach, then the
// docked dwell — the cargo/market action fires partway through it, never
// at the instant of contact — then undock + departure beat before the
// hauler leaves the grid. Each stage arms its own jittered deadline.
func (s *Sim) trafficTurnaround(sys *content.SystemDef, e ecs.Entity, tc *Traffic, dest *content.CelestialDef) {
	switch tc.Phase {
	case TrafficApproach:
		if s.time < tc.Next {
			return
		}
		tc.Phase = TrafficDock
		tc.Next = s.time + float64(trafficBeat(beatKey(e, "dock"),
			trafficDockMin, trafficDockMax))*DT
	case TrafficDock:
		if s.time < tc.Next {
			return
		}
		// Cargo action, partway through the dwell: the shipment posts (and
		// a wish shipment settles its haul) now — not at first contact.
		s.trafficDeliver(sys, e, tc, dest)
		tc.Phase = TrafficBerth
		tc.Next = s.time + float64(trafficBeat(beatKey(e, "berth"),
			trafficBerthMin, trafficBerthMax))*DT
	case TrafficBerth:
		if s.time < tc.Next {
			return
		}
		tc.Phase = TrafficUndock
		tc.Next = s.time + float64(trafficBeat(beatKey(e, "undock"),
			trafficDepartBeatMin, trafficDepartBeatMax))*DT
	case TrafficUndock:
		if s.time < tc.Next {
			return
		}
		s.destroy(e)
	default: // first contact (transit / unknown): arm the approach
		tc.Phase = TrafficApproach
		tc.Next = s.time + float64(trafficBeat(beatKey(e, "approach"),
			trafficApproachMin, trafficApproachMax))*DT
	}
}

// trafficHop completes a cross-system leg: the hauler dwells out its gate
// cycle (see trafficGateCycle — the caller arms TrafficGate first), then
// jumps through the gate it reached and the shipment parks as a journey
// (Save.TrafficJourneys) in the far system — it flies on when that grid
// loads next. The parked entry carries a jump cycle (jittered from the
// same pure hashes): between hop-out and materializing at the destination
// gate there is always a floor, so an arrival is never instantaneous even
// when the player follows straight behind. The far-system leg is a NEW
// leg: its ambush roll is unconsumed (per-leg risk under the destination's
// own security), and any live ambush dies with this grid.
//
// Gate queue: when the far system's parked ledger is already at its
// ceiling (trafficMaxParkedJourneys), the journey HOLDS at its CURRENT
// system instead — same provenance, same final destination, spawn position
// kept where the hauler stands — and the hop is retried on later ticks
// once the far queue drains. A hold never hop-parks past the ceiling, is
// never dropped, and never moves a unit of cargo (closed mass balance);
// backlog therefore pushes upstream across the ring instead of stacking
// unboundedly at one destination.
//
// Everything COMMENCES on the beat (the paced windows below): spawns and
// resumes hold a departure beat, gate arrivals hold a gate cycle
// (dwell → jump), station arrivals run a turnaround (approach → docked
// dwell with the cargo action partway → undock + beat), and a hold logs
// once when its episode begins. Every window is jittered from pure hashes
// of (tick, canonical key) — stateless, entropy-free, schema-free — so the
// delays change only WHEN things happen: the closed mass balance above is
// untouched. The hold announces itself ONCE — on
// entering the hold — because a held shipment then waits IN THE LEDGER
// (the resume loop hops gate-standing entries straight across, see
// trafficLedgerHop): no respawn/re-hold cycle exists to re-log, and the
// successful jump below is the episode's follow-up line.
func (s *Sim) trafficHop(sys *content.SystemDef, e ecs.Entity, tc *Traffic, gate *content.CelestialDef) {
	if gate.Gate == nil {
		// A cross-system leg always ends at a gate (route validation):
		// nothing sane to do otherwise — drop the leg as before.
		s.trafficLoseWish(tc)
		s.destroy(e)
		return
	}
	nextSys := gate.Gate.ToSystem
	nextDef, ok := s.Content.System(nextSys)
	if !ok {
		s.trafficLoseWish(tc)
		s.destroy(e)
		return
	}
	hull := content.ShipID("")
	if sh, ok := s.Ships.Get(e); ok {
		hull = sh.Hull
	}
	if s.trafficParkedIn(nextSys) >= trafficMaxParkedJourneys {
		// Queue full: hold the shipment here (see the doc comment above).
		s.trafficJourneys = append(s.trafficJourneys, TrafficJourney{
			System: sys.ID, At: mustPos(s, e), DestSys: tc.DestSys, DestCel: tc.DestCel,
			Item: tc.Item, Qty: tc.Qty, Basis: tc.Basis, Hull: hull,
			Owner: tc.Owner,
		})
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e,
			System: sys.ID, Cel: gate.ID, Item: tc.Item, Value: float64(tc.Qty),
			Text: "Convoy hauler holds at " + gate.Name + " — " + nextDef.Name +
				" gate queue full, shipment waits in " + sys.Name})
		s.destroy(e)
		return
	}
	entry := sysCelestial(nextDef, gate.Gate.ToCel)
	var at core.Vec2
	if entry != nil {
		at = entry.Position
	}
	s.trafficJourneys = append(s.trafficJourneys, TrafficJourney{
		System: nextSys, At: at, DestSys: tc.DestSys, DestCel: tc.DestCel,
		Item: tc.Item, Qty: tc.Qty, Basis: tc.Basis, Hull: hull,
		Owner: tc.Owner,
		// Jump cycle: the continuation is not due at the destination gate
		// until hop + 1..3 s (resume checks s.time < j.Next first).
		Next: s.time + float64(trafficBeat(beatKey(e, "jump"),
			trafficJumpCycleMin, trafficJumpCycleMax))*DT,
	})
	s.emit(Event{Kind: EvTraffic, Phase: TrafficJumped, Severity: SevInfo, Actor: e,
		System: sys.ID, Cel: gate.ID, Item: tc.Item, Value: float64(tc.Qty),
		Text: "Convoy hauler jumps through " + gate.Name + " to " + nextDef.Name})
	s.destroy(e)
}

// trafficParkedIn counts the journeys parked for a system — its gate
// queue. Ledger order is canonical; this is a pure count (no entropy).
func (s *Sim) trafficParkedIn(sys content.SystemID) int {
	n := 0
	for i := range s.trafficJourneys {
		if s.trafficJourneys[i].System == sys {
			n++
		}
	}
	return n
}

// trafficParkGrid parks every live convoy hauler just before the grid is
// rebuilt (see loadSystem): the shipment keeps its provenance and final
// destination as a journey record, so cargo sourced at spawn can never
// vanish silently with the grid — it flies again when its system loads. A
// live ambush does not survive the reload (its raiders die with the grid);
// the hauler continues its leg, and the roll it already consumed stays
// consumed.
func (s *Sim) trafficParkGrid() {
	for _, e := range sortedEntities(s.Traffic) {
		tc, ok := s.Traffic.Get(e)
		if !ok {
			continue
		}
		tr, ok := s.Transforms.Get(e)
		if !ok {
			continue
		}
		hull := content.ShipID("")
		if sh, ok := s.Ships.Get(e); ok {
			hull = sh.Hull
		}
		// A hauler parked DURING its turnaround's rest has already posted
		// its hold (the cargo action cleared it) — it has no continuation:
		// parking it would forge a journey claiming cargo that is already
		// on the destination book (a ghost shipment that "delivers" twice).
		// The undock beat is cosmetic; the grid rebuild discards the husk.
		if tc.Phase == TrafficBerth || tc.Phase == TrafficUndock {
			continue
		}
		destCel := tc.DestCel
		if destCel == "" { // pre-cross-system Traffic: the leg target was the destination
			destCel = tc.Dest
		}
		s.trafficJourneys = append(s.trafficJourneys, TrafficJourney{
			System: s.systemID, At: tr.Pos, DestSys: tc.DestSys, DestCel: destCel,
			Item: tc.Item, Qty: tc.Qty, Basis: tc.Basis, Hull: hull,
			Rolled: tc.Rolled, Next: tc.Next, Owner: tc.Owner,
		})
	}
}

// trafficResumeJourneys respawns journeys waiting in the active grid,
// behind three stateless gates so a deep ledger can never unload as one
// fleet:
//
//   - jump cycle / carried deadline: a journey whose Traffic.Next is still
//     in the future (the hop's jump cycle, or a stage deadline a grid
//     rebuild froze mid-stage) waits where it is;
//   - live cap: at most (trafficMaxLive − live haulers on this grid) spawns
//     per tick — the same cap the fresh schedules enforce, so first entry
//     into a system shows ordinary traffic density and the backlog drains
//     at the normal cadence behind it;
//   - departure stagger: each ledger entry fires on its own stateless
//     schedule (trafficDue keyed by system + ledger position), so resumed
//     convoys spread across ticks instead of aligning and warping together.
//
// Gate-standing entries — a shipment already standing at its next-hop gate
// (a gate hold, or a park frozen mid-gate-cycle) — never respawn a hauler
// just to jump again: they hop straight from the ledger once the far
// queue has room (trafficLedgerHop), and keep waiting silently while it is
// full. That silence is what makes a hold episode log exactly once — the
// live hauler can only hold on its FIRST arrival; every later retry is a
// ledger operation.
//
// Selection walks the ledger in canonical order; a gated journey simply
// keeps its entry for a later tick (nothing reschedules, and trafficPhase
// is a pure hash — resume draws no entropy, so twin runs and save/restore
// round-trips stay bit-identical). Journeys of other systems stay parked
// (they do not tick while the player is away — the respawn-timer rule
// applies to shipments too). A journey whose route can no longer resolve
// on this grid resolves as a lost shipment (event, no book movement, no
// re-spawn: the origin was debited at spawn) — content is static, so this
// is a backstop.
func (s *Sim) trafficResumeJourneys(sys *content.SystemDef) {
	if len(s.trafficJourneys) == 0 {
		return
	}
	// A gate-standing entry may hop (or keep waiting) only after its own
	// resume schedule fires; classified first, resolved after `kept` is
	// installed so the ceiling counts this tick's hops as they land.
	type gateStanding struct {
		j    TrafficJourney
		i    int
		gate *content.CelestialDef
	}
	budget := trafficMaxLive - s.Traffic.Len()
	kept := make([]TrafficJourney, 0, len(s.trafficJourneys))
	var standing []gateStanding
	for i := range s.trafficJourneys {
		j := s.trafficJourneys[i]
		if j.System != s.systemID {
			kept = append(kept, j)
			continue
		}
		if s.time < j.Next { // jump cycle / a frozen stage deadline
			kept = append(kept, j)
			continue
		}
		destSys := j.DestSys
		if destSys == "" { // pre-cross-system saves: the journey ends here
			destSys = j.System
		}
		target := j.DestCel
		if destSys != s.systemID {
			if target = s.trafficNextHop(s.systemID, destSys); target == "" {
				s.trafficJourneyLost(sys, j, destSys)
				continue
			}
		} else if sysCelestial(sys, target) == nil {
			s.trafficJourneyLost(sys, j, destSys)
			continue
		}
		hull, ok := s.Content.Ship(j.Hull)
		if !ok {
			s.trafficJourneyLost(sys, j, destSys)
			continue
		}
		// Standing at its own next-hop gate: it needs no flight before the
		// hop (the same warpLandRange measure the drive loop uses), so it
		// continues from the ledger — never through a spawn.
		if destSys != s.systemID {
			if gate := sysCelestial(sys, target); gate != nil && gate.Gate != nil &&
				j.At.Dist(gate.Position) <= warpLandRange {
				standing = append(standing, gateStanding{j: j, i: i, gate: gate})
				continue
			}
		}
		if budget <= 0 ||
			!trafficDue(s.ticks, string(s.systemID)+"|resume|"+itoaN(i), trafficResumeInterval) {
			kept = append(kept, j)
			continue
		}
		budget--
		s.spawnResumedHauler(sys, hull, j, target)
	}
	s.trafficJourneys = kept
	for _, h := range standing {
		if !trafficDue(s.ticks, string(s.systemID)+"|resume|"+itoaN(h.i), trafficResumeInterval) {
			s.trafficJourneys = append(s.trafficJourneys, h.j)
			continue
		}
		s.trafficLedgerHop(sys, h.j, h.gate)
	}
}

// trafficLedgerHop jumps a gate-standing journey straight from the ledger
// (see trafficResumeJourneys): no hauler spawns just to jump again, and a
// far queue still at its ceiling keeps the entry here — silently, because
// the hold's one announcement already fired when the episode began. The
// far entry carries the jump cycle's floor (hop → materialize) exactly
// like trafficHop's park.
func (s *Sim) trafficLedgerHop(sys *content.SystemDef, j TrafficJourney, gate *content.CelestialDef) {
	nextSys := gate.Gate.ToSystem
	nextDef, ok := s.Content.System(nextSys)
	if !ok {
		s.trafficJourneyLost(sys, j, nextSys)
		return
	}
	if s.trafficParkedIn(nextSys) >= trafficMaxParkedJourneys {
		s.trafficJourneys = append(s.trafficJourneys, j) // hold continues, quietly
		return
	}
	entry := sysCelestial(nextDef, gate.Gate.ToCel)
	var at core.Vec2
	if entry != nil {
		at = entry.Position
	}
	j.System = nextSys
	j.At = at
	j.Rolled = false // a hop is a NEW leg: the far ambush roll is unconsumed
	j.Next = s.time + float64(trafficBeat(
		string(sys.ID)+"|jump|"+string(j.DestCel)+"|"+string(j.Item),
		trafficJumpCycleMin, trafficJumpCycleMax))*DT
	s.trafficJourneys = append(s.trafficJourneys, j)
	s.emit(Event{Kind: EvTraffic, Phase: TrafficJumped, Severity: SevInfo,
		System: sys.ID, Cel: gate.ID, Item: j.Item, Value: float64(j.Qty),
		Text: "Convoy hauler jumps through " + gate.Name + " to " + nextDef.Name})
}

// spawnResumedHauler re-creates a parked shipment's hauler at its recorded
// position with its provenance intact and announces the continuation. No
// RNG draws: the parked position is exact. The carried Next only gated the
// resume itself — createHauler arms a fresh departure beat, so a
// materialized continuation holds before its onward warp like every spawn.
func (s *Sim) spawnResumedHauler(sys *content.SystemDef, hull *content.ShipDef, j TrafficJourney, target content.CelID) {
	e := s.createHauler(hull, j.At, 0, Traffic{
		Dest: target, DestSys: j.DestSys, DestCel: j.DestCel,
		Item: j.Item, Qty: j.Qty, Basis: j.Basis, Owner: j.Owner,
		Phase: TrafficTransit, Rolled: j.Rolled, Next: j.Next,
	})
	nearID, nearName := s.nearestLandmark(j.At)
	destName := string(j.DestCel)
	if destDef, ok := s.Content.System(j.DestSys); ok {
		destName = celIDOrName(destDef, j.DestCel)
	}
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDeparted, Severity: SevInfo, Actor: e,
		System: sys.ID, Cel: nearID, Item: j.Item, Value: float64(j.Qty),
		Text: "Convoy hauler departs " + nearName + " bound for " + destName})
}

// trafficJourneyLost announces a parked shipment that can no longer
// continue — honest loss: an event, no book movement, no re-spawn. A wish
// shipment goes with it: goods already debited, no payment, wish dropped.
func (s *Sim) trafficJourneyLost(sys *content.SystemDef, j TrafficJourney, destSys content.SystemID) {
	destName := string(j.DestCel)
	if def, ok := s.Content.System(destSys); ok {
		destName = def.Name
	}
	s.emit(Event{Kind: EvMarket, Severity: SevWarning, System: sys.ID, Item: j.Item,
		Value: float64(j.Qty),
		Text: "Risk premium — " + s.itemName(j.Item) +
			" shipment lost en route to " + destName})
	s.trafficLoseJourney(j)
}

// trafficNextHop returns the gate celestial id in `from` that begins the
// shortest stargate path toward `dest` ("" when dest == from or no gate
// path exists). Breadth-first over the gate graph; ties break on celestial
// definition order, so the walk is deterministic.
func (s *Sim) trafficNextHop(from, dest content.SystemID) content.CelID {
	if from == dest || s.Content == nil {
		return ""
	}
	if _, ok := s.Content.System(from); !ok {
		return ""
	}
	if _, ok := s.Content.System(dest); !ok {
		return ""
	}
	// BFS from `from`; parent records the gate that first reached each
	// system (level order ⇒ the parent chain is a shortest path).
	seen := map[content.SystemID]bool{from: true}
	parentSys := map[content.SystemID]content.SystemID{}
	parentGate := map[content.SystemID]content.CelID{}
	queue := []content.SystemID{from}
	for len(queue) > 0 && !seen[dest] {
		u := queue[0]
		queue = queue[1:]
		uDef, ok := s.Content.System(u)
		if !ok {
			continue
		}
		for i := range uDef.Celestials {
			cel := &uDef.Celestials[i]
			if cel.Gate == nil {
				continue
			}
			v := cel.Gate.ToSystem
			if seen[v] {
				continue
			}
			seen[v] = true
			parentSys[v] = u
			parentGate[v] = cel.ID
			queue = append(queue, v)
		}
	}
	if !seen[dest] {
		return ""
	}
	// Walk the parent chain back to `from`: the last step is its gate.
	cur := dest
	for cur != from && parentSys[cur] != from {
		cur = parentSys[cur]
	}
	return parentGate[cur]
}

// trafficDeliver posts the shipment as supply on the destination station's
// book. An AUTHORED shipment lists at its spawn-time cost basis: units
// fulfilled from the origin station's asks were debited when the hauler
// spawned (their weighted price IS the basis), belt and gate (import)
// shipments carry the item's base price — so no withdrawal happens here,
// and a shipment that never arrived needs no reversal (its goods left the
// origin book at spawn). The ask lists one tick under min(cost basis,
// best ask), so arriving supply always improves the displayed price — and
// a posting that crosses the destination's bids executes immediately at
// the resting bid (never-crossed invariant), only the remainder resting.
//
// A WISH shipment (Traffic.Owner set) lists at the agent's OWN ask
// instead — its EWMA belief marked up by its ZIP margin, posted under its
// owner tag — and then settles its wish through FulfillHaul (the pinned
// haul fee). The market voice runs on both.
func (s *Sim) trafficDeliver(sys *content.SystemDef, e ecs.Entity, tc *Traffic, dest *content.CelestialDef) {
	item, qty := tc.Item, tc.Qty
	costBasis := tc.Basis
	if costBasis <= 0 {
		// Pre-provenance saves carry no basis: fall back to the ladder's
		// last rung, exactly as a belt/gate shipment would.
		costBasis = s.marketBasePrice(item)
	}
	book := s.market[dest.ID]
	before := s.marketPrice(book, item)
	agent := s.agentByOwner(tc.Owner)
	if dest.Station != nil && hasMarketService(dest.Station) {
		agentAsk := 0.0
		if agent != nil {
			agentAsk = s.agentHaulAsk(agent, item)
		}
		if agentAsk > 0 {
			// Wish shipment: the cargo lists at the agent's reservation
			// ask (belief × margin), owned by the agent — not one tick
			// under the convoy's basis.
			s.haulPlaceAgentAsk(agent, dest.ID, item, qty, agentAsk)
		} else {
			price := costBasis
			if book != nil {
				if ask, ok := book.bestAsk(item); ok && ask.Price < price {
					price = ask.Price
				}
			}
			if post := tickDown(price); post > 0 {
				s.PlaceOrder(dest.ID, OrderSell, item, qty, post, OwnerSystem)
			}
		}
		book = s.market[dest.ID] // placement may have created the book
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
	s.marketVoice(book, item, before, dest.Name)
	if agent != nil {
		s.trafficSettleWish(agent, tc, dest.ID)
	}
	// The cargo action offloads the hold: the units posted above ARE the
	// shipment, so from here the standing hauler carries nothing (the
	// closed mass balance counts cargo once — in the book, never in the
	// hold as well — and a kill during the rest of the turnaround wrecks
	// an empty ship).
	if c, ok := s.Cargos.Get(e); ok {
		c.Stacks = nil
		s.Cargos.Set(e, *c)
	}
	// The hauler does NOT leave here: trafficTurnaround keeps it standing
	// through the rest of the docked dwell and despawns it after the
	// undock departure beat.
}

// trafficHaulerDestroyed resolves a lost convoy hauler (raider kill or
// player kill): the shipment turns into a lootable wreck (the caller adds
// the cargo — the goods sourced at spawn are destroyed unless someone
// loots them back) and the loss is announced as an event-only supply-risk
// signal. No reversal and no re-spawn: the origin book was debited when
// the hauler spawned, and a shipment that never arrived never touched the
// destination book.
func (s *Sim) trafficHaulerDestroyed(e ecs.Entity, tc *Traffic) {
	// A hauler killed DURING its turnaround's rest (post-delivery: the
	// cargo action already posted the hold and settled the wish) is just a
	// lost ship — the shipment arrived, so no risk premium speaks and the
	// wreck carries nothing. Pre-delivery deaths keep every loss signal.
	delivered := tc.Phase == TrafficBerth || tc.Phase == TrafficUndock
	// A wish shipment dies with its hauler: the goods were debited at
	// spawn (they ride the wreck), so the agent is never paid and its
	// wish is dropped — the events below are the existing loss signals.
	if !delivered {
		s.trafficLoseWish(tc)
	}
	destName := ""
	if sys := s.System(); sys != nil {
		if d := sysCelestial(sys, tc.Dest); d != nil {
			destName = d.Name
		}
	}
	if destName != "" && !delivered {
		s.emit(Event{Kind: EvMarket, Severity: SevWarning, System: s.systemID, Item: tc.Item,
			Value: float64(tc.Qty),
			Text: "Risk premium — " + s.itemName(tc.Item) +
				" shipment lost en route to " + destName})
	}

	nearID, nearName := s.nearestLandmark(mustPos(s, e))
	cargoTail := " with its cargo"
	if delivered {
		cargoTail = ""
	}
	s.emit(Event{Kind: EvTraffic, Phase: TrafficDestroyed, Severity: SevDanger, Actor: e,
		System: s.systemID, Cel: nearID, Item: tc.Item, Value: float64(tc.Qty),
		Text: "Convoy hauler destroyed" + cargoTail + " near " + nearName})
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
	var c btCtx
	for _, r := range atk.Raiders {
		if !s.World.Alive(r) {
			continue
		}
		ai, ok := s.AIs.Get(r)
		if !ok {
			continue
		}
		c = btCtx{s: s, ai: ai, atk: atk, hauler: hauler}
		c.e = r
		c.role = "raider"
		raiderTree.Tick(&c)
		c.done()
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

// trafficRespond dispatches the security response: a wing of
// trafficPatrolWing(σ) ships (a full ten in high-sec, a single late
// responder in low, nothing in null), spawned at the anchor when the wing is
// under cap, otherwise reassigned from an idle patrol already on duty. The
// incident is an ambushed convoy hauler or a miner under fire (minerDistress):
// a miner's wing waits out trafficResponseDelay at the anchor before it warps
// (an ambush already carries the delay in RespondAt).
func (s *Sim) trafficRespond(sys *content.SystemDef, incident ecs.Entity, td *content.TrafficDef, sec string) {
	if td == nil || td.Patrol == nil || len(td.Patrol.Hulls) == 0 {
		return
	}
	wing := trafficPatrolWing(sec)
	if wing <= 0 {
		return
	}
	miner := s.isMinerIncident(incident)
	what := "the ambush"
	delay := int64(0)
	if miner {
		what = "the miner under fire"
		delay = trafficResponseDelay(sec)
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
			p.Incident = incident
			p.Next = s.time + float64(delay)*DT
		}
		dispatched++
	}
	if dispatched == 0 {
		return
	}
	pos := mustPos(s, incident)
	celID, celName := s.nearestLandmark(pos)
	s.emit(Event{Kind: EvTraffic, Phase: TrafficPatrolResponse, Severity: SevGood,
		System: sys.ID, Cel: celID, Value: float64(dispatched),
		Text: "Patrol wing responding to " + what + " near " + celName})
}

// minerDistress dispatches the Charter Authority's answer to a miner under
// fire: the same wing as a convoy ambush, held on the miner and the rats
// around it, and only one live response per miner (a new dispatch after that
// wing left or died and the miner is hit again). Works-held systems get none
// — charter security does not answer inside Works control — and null space
// has no wing to send. Any attacker calls it: the authority answers distress,
// not only the Old Works.
func (s *Sim) minerDistress(miner ecs.Entity) {
	sys := s.System()
	if sys == nil || (s.war != nil && s.war.Control[sys.ID] == controlWorks) {
		return
	}
	if s.respondingTo(miner) {
		return
	}
	td, ok := s.Content.TrafficFor(sys.ID)
	if !ok || td.Patrol == nil || len(td.Patrol.Hulls) == 0 {
		return
	}
	sec := s.EffectiveSecurity(sys.ID)
	if trafficPatrolWing(sec) <= 0 {
		return
	}
	s.trafficRespond(sys, miner, td, sec)
}

// respondingTo reports whether a live patrol is already working incident e.
func (s *Sim) respondingTo(e ecs.Entity) bool {
	for _, p := range sortedEntities(s.Patrols) {
		if st, ok := s.Patrols.Get(p); ok && st.Incident == e &&
			st.State == PatrolRespond && s.World.Alive(p) {
			return true
		}
	}
	return false
}

// isMinerIncident reports whether e is a live civilian miner — the incident
// kind a distress wing holds on.
func (s *Sim) isMinerIncident(e ecs.Entity) bool {
	if e.IsZero() || !s.World.Alive(e) {
		return false
	}
	ai, ok := s.AIs.Get(e)
	if !ok {
		return false
	}
	def, ok := s.Content.Npc(ai.Npc)
	return ok && def.IsMiner()
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
// the anchor and resume the idle guard circle. It also dispatches idle
// patrols onto occupied high-sec belts (patrolSweepDispatch). Only idle AI
// states are driven here — an aggroed patrol belongs to systemNpc.
func (s *Sim) trafficDrivePatrols(sys *content.SystemDef, td *content.TrafficDef, sec string) {
	var c btCtx
	for _, e := range sortedEntities(s.Patrols) {
		p, ok := s.Patrols.Get(e)
		if !ok {
			continue
		}
		ai, hasAI := s.AIs.Get(e)
		if !hasAI {
			continue
		}
		c = btCtx{s: s, e: e, ai: ai, p: p, security: sec, sys: sys, td: td}
		c.role = "patrol"
		patrolTree.Tick(&c)
		c.done()
	}
}

// trafficBeltSweepBeat is the patrol beat (ticks) between belt-sweep
// considerations: the authored patrol cadence scaled by security exactly like
// the patrol spawn schedule (trafficPatrolInterval) — a patrol answers a belt
// when its beat comes round, not the tick the belt is first occupied, so a
// pilot already on the belt keeps first claim on the rats. 0 when the system
// has no patrol definition (or null space, where no patrol flies).
func trafficBeltSweepBeat(td *content.TrafficDef, sec string) int64 {
	if td == nil || td.Patrol == nil {
		return 0
	}
	interval, ok := trafficPatrolInterval(td.Patrol.IntervalTicks, sec)
	if !ok {
		return 0
	}
	return interval
}

// patrolSweepDispatch sends an idle patrol to a high-sec belt holding live,
// unmolested rats, one beat at a time. Patrol.Next carries the beat deadline
// (the field already exists for the warp-retry cooldown, so the save schema is
// unchanged); the decision itself is recomputed from world state every beat,
// and the sweep re-arms after it returns home. Low/null space gets no sweep.
func (s *Sim) patrolSweepDispatch(p *Patrol, sys *content.SystemDef, td *content.TrafficDef, sec string) {
	beat := trafficBeltSweepBeat(td, sec)
	if beat <= 0 {
		return // no patrol schedule here: nothing to sweep with
	}
	if p.Next == 0 {
		// Coming on duty (or back from an incident): the first sweep
		// consideration waits one full beat.
		p.Next = s.time + float64(beat)*DT
		return
	}
	if s.time < p.Next {
		return
	}
	p.Next = s.time + float64(beat)*DT
	if sec != "high" || sys == nil {
		return
	}
	if belt := s.occupiedBelt(sys, sec); !belt.IsZero() {
		p.State = PatrolRespond
		p.Incident = belt
		p.Next = 0
	}
}

// occupiedBelt returns the celestial entity of the first belt in the system
// (celestial order) holding any live rat, or zero. Player-touched rats still
// occupy the belt: the sweep is sent to finish it, free rats first.
func (s *Sim) occupiedBelt(sys *content.SystemDef, sec string) ecs.Entity {
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		if cel.Kind != content.CelBelt || s.ratTargetAt(cel.Position, sec).IsZero() {
			continue
		}
		return s.celEntity(cel.ID)
	}
	return 0
}

// patrolStation returns the station point a responder holds on the ring
// around centre: responders to the same incident get even angular slots in
// handle order (a ten-ship wing is then at least
// 2*patrolStationRMin*sin(π/10) ≈ 1.1 km apart, so no two hulls stack), and
// the radius varies with the handle hash (patrolHash, no RNG draw) so the
// ring never reads as a drawn circle. Recomputed on each warp, never stored
// (no save-schema change), so the layout is a pure function of world state.
func (s *Sim) patrolStation(e, incident ecs.Entity, centre core.Vec2) core.Vec2 {
	slot, n := 0, 0
	for _, o := range sortedEntities(s.Patrols) {
		p, ok := s.Patrols.Get(o)
		if !ok || p.State != PatrolRespond || p.Incident != incident || !s.World.Alive(o) {
			continue
		}
		if o == e {
			slot = n
		}
		n++
	}
	if n == 0 {
		n = 1 // not in the table yet: its own slot 0 ring
	}
	a := 2 * math.Pi * float64(slot) / float64(n)
	r := patrolStationRMin + (patrolStationRMax-patrolStationRMin)*patrolPhase(e)/(2*math.Pi)
	return centre.Add(core.Vec2{X: math.Cos(a) * r, Y: math.Sin(a) * r})
}

// patrolRespondStep advances one responding patrol toward and into the
// fight, or sends it home when the raid is over. A belt incident (the
// celestial entity of an occupied high-sec belt) is a sweep instead, and a
// miner incident is a distress hold (patrolMinerStep).
func (s *Sim) patrolRespondStep(e ecs.Entity, p *Patrol, ai *NpcAI, sec string) {
	if c, ok := s.Celestials.Get(p.Incident); ok && c.Kind == content.CelBelt {
		s.patrolSweepStep(e, p, ai, sec)
		return
	}
	if s.isMinerIncident(p.Incident) {
		s.patrolMinerStep(e, p, ai)
		return
	}
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
		// The sortie warps to the patrol's STATION on the ring around the
		// incident (the ambushed hauler's position), not to a live raider: a
		// raider mid-warp-in moves faster than any lock and would drag the
		// patrol into stale-point hop chasing. The ring spreads the wing so
		// ten hulls do not land on one point; the raiders land on the same
		// fight grid, so the first warp still closes to engagement range in
		// one align + tunnel (the ring is well inside it). warpToPoint lands
		// exactly on the station, and the hold below keeps the ship there.
		if ip, ok := s.posOf(p.Incident); ok && s.time >= p.Next &&
			s.warpToPoint(e, s.patrolStation(e, p.Incident, ip)) {
			p.Next = s.time + trafficWarpRetrySec
		}
		return
	}
	s.retargetWeapons(e, target)
	s.npcEnsureLock(e, target)
	s.npcActivate(e, ai, target)
}

// patrolSweepStep advances one patrol sweeping an occupied belt: it hunts the
// next rat (free ones first, then any the player left) — warping out to a
// straggler beyond warp minimum range, thrusting after one closer — then
// locks and shoots it. It goes home only when the belt is empty
// (patrolReturnStep flies it back). Rats respawn on their own timers, so the
// next beat re-arms the sweep — ongoing skirmishes, no suppression.
func (s *Sim) patrolSweepStep(e ecs.Entity, p *Patrol, ai *NpcAI, sec string) {
	if s.Warps.Has(e) {
		return // warp state machine owns movement
	}
	// Hold station while in range: systemNpc skips the idle-patrol circle
	// while the patrol is responding, and a stale patrol intent must not
	// steer it away from the rats (same rule as an ambush response).
	s.Intents.Set(e, Intent{Kind: IntentStop})
	bp, ok := s.posOf(p.Incident)
	if !ok {
		p.State = PatrolReturn
		p.Incident = 0
		p.Next = 0
		return
	}
	target := s.ratTargetAt(bp, sec)
	if target.IsZero() {
		p.State = PatrolReturn
		p.Incident = 0
		p.Next = 0
		return
	}
	ep, ok := s.posOf(e)
	if !ok {
		return
	}
	tp, ok := s.posOf(target)
	if !ok {
		return
	}
	if ep.Dist(tp) > trafficPatrolEngageRange {
		// Hunt the straggler: warp out to it when it is beyond warp minimum
		// range (the patrolRespondStep vocabulary), else close under thrust.
		if ep.Dist(tp) >= WarpMinRange && s.time >= p.Next && s.warpToPoint(e, tp) {
			p.Next = s.time + trafficWarpRetrySec
			return
		}
		s.Intents.Set(e, Intent{Kind: IntentApproach, Target: target})
		return
	}
	s.retargetWeapons(e, target)
	s.npcEnsureLock(e, target)
	s.npcActivate(e, ai, target)
	s.ratDefend(target, e)
}

// patrolMinerStep holds a distress wing on a miner under fire: it waits out
// the response delay at the anchor, warps to the miner, shoots the rats
// within trafficPatrolEngageRange of it (the same range it engages an
// ambush at), and flies home when none are left. Rats only: a guard never
// initiates on the player (systemNpc's player-only self-defence), so the
// wing answers the miner's attackers and nothing else.
func (s *Sim) patrolMinerStep(e ecs.Entity, p *Patrol, ai *NpcAI) {
	if !s.isMinerIncident(p.Incident) {
		p.State = PatrolReturn
		p.Incident = 0
		p.Next = 0
		return
	}
	if s.Warps.Has(e) {
		return // warp state machine owns movement
	}
	mp, ok := s.posOf(p.Incident)
	if !ok {
		return
	}
	ep, ok := s.posOf(e)
	if !ok {
		return
	}
	if s.time < p.Next {
		// The response delay (low-sec): hold at the anchor, then fly.
		s.Intents.Set(e, Intent{Kind: IntentStop})
		return
	}
	if ep.Dist(mp) > trafficPatrolEngageRange {
		// Warp to the wing slot on the ring around the barge: the wing holds
		// a formation over the miner, never a stack on its hull. The ring is
		// inside the engage range, so the rats stay in reach.
		if s.warpToPoint(e, s.patrolStation(e, p.Incident, mp)) {
			p.Next = s.time + trafficWarpRetrySec
		}
		return
	}
	// Hold station on the miner while the rats around it are cleared.
	s.Intents.Set(e, Intent{Kind: IntentStop})
	target := s.ratNear(mp, trafficPatrolEngageRange)
	if target.IsZero() {
		p.State = PatrolReturn
		p.Incident = 0
		p.Next = 0
		return
	}
	s.retargetWeapons(e, target)
	s.npcEnsureLock(e, target)
	s.npcActivate(e, ai, target)
	s.ratDefend(target, e)
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
		p.Next = 0 // a fresh beat starts when the patrol is back on duty
		return
	}
	if s.time >= p.Next && s.warpToPoint(e, ai.Home) {
		p.Next = s.time + trafficWarpRetrySec
	}
}

// ---------------------------------------------------------------------------
// Market hooks — the book does the pricing; traffic only posts supply
// ---------------------------------------------------------------------------

// marketBasePrice returns the reference (def base) price of an item: the
// item's base price, the module's authored price, else the hull's base
// price. It is the price ladder's last rung and a shipment's cost basis
// when its origin carries no book to draw from.
func (s *Sim) marketBasePrice(item content.ItemID) float64 {
	if def, ok := s.Content.Item(item); ok {
		return def.BasePrice
	}
	if def, ok := s.Content.Module(content.ModuleID(item)); ok {
		return def.Price
	}
	if def, ok := s.Content.Ship(content.ShipID(item)); ok {
		return def.BasePrice
	}
	return 0
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

// trafficLiveRaiders counts the traffic raiders alive on this grid: every
// living raider referenced by a live ambush (whatever its outcome — the
// raiders linger until break-off after an escape) plus scheduled break-offs
// still on the grid (a hauler that dies hands its raiders to trafficLeaves).
// The overlap between the two sets is de-duplicated by handle; the map is
// only measured with len(), never iterated, so no map order reaches
// behavior. Static spawnGroup NPCs and patrols are not raiders and never
// count — this is the population the trafficMaxRaiders ceiling governs.
func (s *Sim) trafficLiveRaiders() int {
	seen := make(map[ecs.Entity]bool)
	s.Traffic.Each(func(_ ecs.Entity, tc *Traffic) {
		if tc.Attack == nil {
			return
		}
		for _, r := range tc.Attack.Raiders {
			if s.World.Alive(r) {
				seen[r] = true
			}
		}
	})
	for _, l := range s.trafficLeaves {
		if s.World.Alive(l.Entity) {
			seen[l.Entity] = true
		}
	}
	return len(seen)
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

// celIDOrName returns a celestial's display name inside sys (its ID when
// the celestial no longer exists).
func celIDOrName(sys *content.SystemDef, id content.CelID) string {
	if cel := sysCelestial(sys, id); cel != nil {
		return cel.Name
	}
	return string(id)
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

// sortStationIDs orders station IDs ascending — the canonical iteration
// order over the per-station market books (deterministic map iteration).
func sortStationIDs(ids []StationID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
