package sim

// ---------------------------------------------------------------------------
// The Long Attrition — the war (CONTEXT.md: war, front, contest, squeeze
// curve, procurement, war treasury, effective security, remnant)
//
// Corporate fleets under the Commission fight the Old Works for territory.
// Control is the scoreboard: every system is charter- or works-held, derived
// from authored security at tick 0 and moved only by a contest flip. The
// contiguous front (systems gated to enemy control) carries one meter each,
// pushed by supply delivered to the war and by kill attrition, eroded by the
// Works' pressure scaled by the squeeze curve. The war mints nothing: the
// Corporate Fleets' treasury buys materiel for its base price and the goods
// are consumed into the war — the sink the economy was missing. See
// docs/SIM.md (wave 3) and sim/war_test.go for the pinned behavior.
// ---------------------------------------------------------------------------

import (
	"math"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// Control sides (WarState.Control values).
const (
	controlCharter = "charter"
	controlWorks   = "works"
)

// War tuning (the owner-tunable knobs of the design package's §6).
const (
	// warEndowment seeds the bloc treasury once, the faction-ledger pattern.
	// It is refilled only by the fee cut, so it is a finite sink.
	warEndowment = 100_000_000.0
	// procureCadence is the war beat (one sim hour): meters settle, bids
	// refresh and fleets look for the front. Supply lands on the next beat,
	// which is the abstract haul delay (≤ one hour, no convoy state).
	procureCadence = int64(3600)
	// feeCut is the share of every broker fee and sales tax routed into the
	// treasury instead of burned; at the 5.5% institutional load that is
	// ~1.1% of turnover.
	feeCut = 0.2
	// newsThreshold is the contest movement that earns a news line.
	newsThreshold = 0.05
	// worksPressureHours is how long unsupplied works pressure takes to
	// flip a charter front at squeeze 1.
	worksPressureHours = 96.0
	// supplyPerFlip is the euro of materiel that moves a meter from 0 to
	// its threshold at squeeze 1 (kills are credited in the same unit).
	supplyPerFlip = 250_000.0
	// killAttrition is the share of a meter one killed Works hull is worth.
	killAttrition = 0.02
	// curveExp shapes the squeeze curve; 1 is flat, higher is steeper.
	curveExp = 2.0
	// squeezeFloor keeps the curve finite at the works floor (resistance is
	// astronomically large there, so the last system can never be taken).
	squeezeFloor = 0.01
	// pressureCap ceilings the works' OFFENSIVE squeeze; their defensive
	// stiffness (what charter supply must beat) stays uncapped, so a
	// cornered Works hits harder but its last system never falls.
	pressureCap = 8.0
	// warWorksFloor is the number of systems the Works always keep: the
	// equilibrium guard — cornered, never extinguished.
	warWorksFloor = 1
	// meterThreshold is the meter value that flips a system.
	meterThreshold = 1.0
	// warBidQty / warBidModuleQty are the unit lots of one war bid line.
	warBidQty       = 100
	warBidModuleQty = 5
	// warFleetCap caps live Corporate Fleets hulls in the loaded system.
	warFleetCap = 2
)

// warWorlds is the world gate (the agentWorlds pattern): the war runs in the
// shipping free-play world. The showcase tour and the test fixtures run
// without it, so their economies cannot drift; waves 2/3 wire the showcase.
var warWorlds = map[content.WorldID]bool{
	"world-default": true,
}

// WarState is the war's persisted state (Save.War): which side holds every
// system, the contest meters on the front, and the Corporate Fleets' bloc
// treasury (per-corp ledgers are a later content change). Squeeze is DERIVED
// from Control and never stored.
type WarState struct {
	Control  map[content.SystemID]string   `json:"control"`
	Contest  map[content.SystemID]*Contest `json:"contest,omitempty"`
	Treasury float64                       `json:"treasury"`
}

// Contest is one front system's meter: charter progress in [0, Threshold],
// pushed by SupplyPending (supply + kill attrition, both in euro-equivalent)
// and eroded by the Works' pressure. The sign of the push follows the holder:
// on a works-held front progress takes the system, on a charter-held front it
// holds it.
type Contest struct {
	Meter         float64 `json:"meter"`
	Threshold     float64 `json:"threshold"`
	SupplyPending float64 `json:"supply_pending,omitempty"`
}

// ---------------------------------------------------------------------------
// Read surface (UI/render)
// ---------------------------------------------------------------------------

// War returns the war state, or nil in a world that carries no war.
func (s *Sim) War() *WarState { return s.war }

// WarTreasury is the Corporate Fleets' bloc balance — the war's health.
func (s *Sim) WarTreasury() float64 {
	if s.war == nil {
		return 0
	}
	return s.war.Treasury
}

// Squeeze is the Works' resistance multiplier: 1 while they hold their tick-0
// share, rising as their map shrinks (the equilibrium guard on the meter, the
// spawn-mix input on the grid).
func (s *Sim) Squeeze() float64 { return s.squeeze() }

// EffectiveSecurity is the one authority every behavior derives from: the
// authored tier while the charter holds the system, "null" once the Works take
// it (guards, patrols, ambush rates, temperament and belt sweeps all follow).
// A reclaim returns the authored tier — nothing about it is stored.
func (s *Sim) EffectiveSecurity(id content.SystemID) string {
	sys, ok := s.Content.System(id)
	if !ok {
		return "null" // fail closed: an unknown system has no law
	}
	if s.war != nil && s.war.Control[id] == controlWorks {
		return "null"
	}
	return sys.Security
}

// ---------------------------------------------------------------------------
// Seeding + geography
// ---------------------------------------------------------------------------

// newWarState seeds control from authored security — the tick-0 map, exactly
// today's — and derives the first front. The treasury starts at the bloc
// endowment (the faction-ledger seed), never lazily.
func (s *Sim) newWarState() *WarState {
	w := &WarState{
		Control:  make(map[content.SystemID]string, len(s.Content.Systems)),
		Treasury: warEndowment,
	}
	for id, sys := range s.Content.Systems {
		w.Control[id] = warSide(sys.Security)
	}
	s.war = w
	s.warRederive()
	return w
}

// warSide maps an authored security tier onto the side that holds the system
// at tick 0: high and low space are chartered, null space is the Works'.
func warSide(security string) string {
	if security == "null" {
		return controlWorks
	}
	return controlCharter
}

// warRederive rebuilds the contest map: every system gated to a system the
// other side holds. Meters carry over for systems that stay on the front, so
// a re-derived geography never loses the pressure already applied; a system
// that just flipped starts idle on its new side.
func (s *Sim) warRederive() {
	if s.war == nil {
		return
	}
	next := make(map[content.SystemID]*Contest, len(s.war.Contest))
	for _, id := range s.warSystemIDs() {
		sys := s.Content.Systems[id]
		if sys == nil || !s.warBordersEnemy(sys, s.war.Control[id]) {
			continue
		}
		if c := s.war.Contest[id]; c != nil {
			next[id] = c
			continue
		}
		next[id] = &Contest{Threshold: meterThreshold}
	}
	s.war.Contest = next
}

// warBordersEnemy reports whether sys has a gate to a system the other side
// holds — the front rule (contiguous, across gates).
func (s *Sim) warBordersEnemy(sys *content.SystemDef, side string) bool {
	if side == "" {
		return false
	}
	for _, cel := range sys.Celestials {
		if cel.Gate == nil {
			continue
		}
		if other := s.war.Control[cel.Gate.ToSystem]; other != "" && other != side {
			return true
		}
	}
	return false
}

// squeeze is the Works' resistance multiplier: 1 at (or above) their tick-0
// share, rising exponentially as they are cornered, floored just short of the
// floor so the last system is asymptotically unbreakable but finite.
func (s *Sim) squeeze() float64 {
	if s.war == nil {
		return 1
	}
	start, works := 0, 0
	for id, sys := range s.Content.Systems {
		if sys.Security == "null" {
			start++
		}
		if s.war.Control[id] == controlWorks {
			works++
		}
	}
	if start <= warWorksFloor || works >= start {
		return 1
	}
	excess := float64(works-warWorksFloor) / float64(start-warWorksFloor)
	if excess < squeezeFloor {
		excess = squeezeFloor
	}
	return math.Exp(curveExp * (1/excess - 1))
}

// warSystemIDs returns every content system id in ascending order.
func (s *Sim) warSystemIDs() []content.SystemID {
	ids := make([]content.SystemID, 0, len(s.Content.Systems))
	for id := range s.Content.Systems {
		ids = append(ids, id)
	}
	sortSystemIDs(ids)
	return ids
}

// warContestIDs returns the front in ascending system id order (canonical
// iteration: map order never reaches behavior).
func (s *Sim) warContestIDs() []content.SystemID {
	ids := make([]content.SystemID, 0, len(s.war.Contest))
	for id := range s.war.Contest {
		ids = append(ids, id)
	}
	sortSystemIDs(ids)
	return ids
}

// ---------------------------------------------------------------------------
// The beat
// ---------------------------------------------------------------------------

// systemWar runs the war's hourly beat (system 19): settle the meters,
// refresh the standing war bids, and look for a fleet action at the front.
// Only the loaded system ticks — like every other system, the war's grid-side
// effects need the player here; the meters themselves are made of state.
func (s *Sim) systemWar() {
	if s.war == nil || s.ticks%procureCadence != 0 {
		return
	}
	s.warContestBeat()
	s.warRefreshBids()
	s.warFleets()
}

// warContestBeat applies one hour of war to every front meter: pending supply
// lands (the abstract haul leg is done), the Works press, and a meter at its
// threshold flips its system.
func (s *Sim) warContestBeat() {
	sq := s.Squeeze()
	pressure := float64(procureCadence) / (worksPressureHours * 3600)
	pressure *= math.Min(sq, pressureCap)
	for _, id := range s.warContestIDs() {
		c := s.war.Contest[id]
		if c == nil {
			continue
		}
		before := c.Meter
		supply := c.SupplyPending / (supplyPerFlip * sq)
		c.SupplyPending = 0
		if s.war.Control[id] == controlCharter {
			c.Meter += pressure - supply // the Works push, the charter holds
		} else {
			c.Meter += supply - pressure // the charter pushes, the Works resist
		}
		c.Meter = math.Min(math.Max(c.Meter, 0), c.Threshold)
		if c.Meter >= c.Threshold {
			s.warFlip(id)
			continue
		}
		if math.Abs(c.Meter-before) >= newsThreshold {
			s.warNewsContest(id, c.Meter)
		}
	}
}

// warCredit books charter push at a front: delivered materiel or destroyed
// Works hulls, in euro-equivalent, released on the next beat.
func (s *Sim) warCredit(sys content.SystemID, value float64) {
	if s.war == nil || value <= 0 {
		return
	}
	if c := s.war.Contest[sys]; c != nil {
		c.SupplyPending += value
	}
}

// warFlip hands a front system to the other side: control, geography, the
// news line, and — when the player is standing in it — the grid itself.
func (s *Sim) warFlip(id content.SystemID) {
	from := s.war.Control[id]
	to := controlCharter
	if from == controlCharter {
		to = controlWorks
	}
	s.war.Control[id] = to
	delete(s.war.Contest, id)
	s.warClearBids(id) // a fallen station's book carries no war demand
	s.warRederive()

	name := s.warSystemName(id)
	if to == controlWorks {
		s.emit(Event{Kind: EvWar, Phase: WarFlip, Severity: SevDanger, System: id,
			Text: "Fall — " + name + " lost to the Old Works"})
	} else {
		s.emit(Event{Kind: EvWar, Phase: WarFlip, Severity: SevGood, System: id,
			Text: "Reclaimed — " + name + " returns to the charter"})
	}
	if id == s.systemID {
		s.warApplyControl(id)
	}
}

// warNewsContest is the front-movement line: only moves of newsThreshold or
// more are reported, so quiet stretches stay quiet.
func (s *Sim) warNewsContest(id content.SystemID, meter float64) {
	s.emit(Event{Kind: EvWar, Phase: WarContest, Severity: SevInfo, System: id,
		Value: meter, Text: "Contested — " + s.warSystemName(id) + " holds at " +
			itoaN(int(math.Round(meter*100))) + "%"})
}

// warSystemName is a system's display name ("?" for a stale id — saves never
// carry one, but a content edit could).
func (s *Sim) warSystemName(id content.SystemID) string {
	if sys, ok := s.Content.System(id); ok {
		return sys.Name
	}
	return string(id)
}

// warApplyControl makes the loaded grid match a flip. Fall: the charter's
// ships leave outright (guards and patrols despawn — no retreat flight this
// wave) and the Works land their belt profile. Reclaim: the charter's guard
// posts come back and the Works hulls already on the grid are left to be
// shot, never silently swept.
func (s *Sim) warApplyControl(id content.SystemID) {
	sys := s.Content.Systems[id]
	if sys == nil {
		return
	}
	if s.war.Control[id] == controlWorks {
		for _, e := range sortedEntities(s.AIs) {
			if s.factionOf(e) == factionCharter {
				s.destroy(e)
			}
		}
		s.warSpawnWorks(sys)
		return
	}
	s.warSpawnGuards(sys)
}

// warSpawnGuards puts the system's authored guard posts back (reclaim).
func (s *Sim) warSpawnGuards(sys *content.SystemDef) {
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		for _, sp := range cel.Spawns {
			def, ok := s.Content.Npc(sp.NpcID)
			if !ok || !def.IsGuard() {
				continue
			}
			for n := 0; n < sp.Count; n++ {
				s.warSpawn(def.ID, cel.Position, celestialSpread(cel), "")
			}
		}
	}
}

// warSpawnWorks lands the Works' holding force in a fallen system: the
// derived profile at the belts and anomalies (never the docking ring), the
// caste and the count both scaled by the squeeze curve.
func (s *Sim) warSpawnWorks(sys *content.SystemDef) {
	castes := warWorksCastes(s.Content)
	if len(castes) == 0 {
		return
	}
	sq := s.Squeeze()
	npc := castes[warProfileTier(sq, len(castes))]
	count := warProfileCount(sq)
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		if cel.Kind != content.CelBelt && cel.Kind != content.CelAnomaly {
			continue
		}
		for k := 0; k < count; k++ {
			s.warSpawn(npc, cel.Position, celestialSpread(cel), "")
		}
	}
}

// warProfileCount is the holding force per anchor: 2 hulls at squeeze 1, one
// more per squeeze step, capped at 4.
func warProfileCount(sq float64) int {
	return 1 + int(math.Min(sq, 3))
}

// warProfileTier picks the Works caste for a squeeze: 1 is the weakest
// authored caste, one caste tougher per squeeze step, capped at the toughest.
func warProfileTier(sq float64, n int) int {
	step := int(sq) - 1
	if step < 0 {
		step = 0
	}
	idx := n - 1 - step
	if idx < 0 {
		idx = 0
	}
	return idx
}

// warWorksCastes ranks the authored Old Works hulls by toughness (total hit
// points, ties by id ascending) — the deterministic palate the holding force
// and the fleets are drawn from. No stat is edited: the war only picks winners
// from what content already ships.
func warWorksCastes(c *content.Content) []content.NpcID {
	type caste struct {
		id content.NpcID
		hp float64
	}
	var all []caste
	for id, def := range c.NPCs {
		if def.Faction != factionOldWorks {
			continue
		}
		hp := 0.0
		if ship, ok := c.Ship(def.Ship); ok {
			hp = ship.Shield + ship.Armor + ship.Structure
		}
		all = append(all, caste{id, hp})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].hp != all[j].hp {
			return all[i].hp > all[j].hp
		}
		return all[i].id < all[j].id
	})
	ids := make([]content.NpcID, len(all))
	for i := range all {
		ids[i] = all[i].id
	}
	return ids
}

// warSpawn lands one war hull on the loaded grid, off the living-universe
// stream so war activity never shifts the player's combat rolls. A faction
// tag overrides the hull's authored colours (the Corporate Fleets fly machine
// hulls content has not named yet).
func (s *Sim) warSpawn(id content.NpcID, pos core.Vec2, spread float64, faction string) ecs.Entity {
	e := s.spawnNpcStream(id, pos, spread, s.trafficRNG)
	if !e.IsZero() && faction != "" {
		if ai, ok := s.AIs.Get(e); ok {
			ai.Faction = faction
		}
	}
	return e
}

// ---------------------------------------------------------------------------
// Fleets
// ---------------------------------------------------------------------------

// warFleets puts a Corporate Fleets group over a contested system: the
// toughest authored machine hull, at the belt the Works are working (or the
// patrol anchor when they are not). They fight through the ordinary threat
// machine — the hostility row does the rest.
func (s *Sim) warFleets() {
	sys := s.System()
	if sys == nil {
		return
	}
	if _, contested := s.war.Contest[sys.ID]; !contested {
		return
	}
	if s.warFleetCount() >= warFleetCap {
		return
	}
	castes := warWorksCastes(s.Content)
	if len(castes) == 0 {
		return
	}
	pos, spread := s.warFleetAnchor(sys)
	s.warSpawn(castes[0], pos, spread, factionFleets)
}

// warFleetCount counts live Corporate Fleets hulls (the cap is per grid).
func (s *Sim) warFleetCount() int {
	n := 0
	for _, e := range sortedEntities(s.AIs) {
		if s.factionOf(e) == factionFleets {
			n++
		}
	}
	return n
}

// warFleetAnchor is where a fleet lands: the occupied belt (the Works' own
// ground), else the patrol anchor the response wing uses.
func (s *Sim) warFleetAnchor(sys *content.SystemDef) (core.Vec2, float64) {
	if belt := s.occupiedBelt(sys, s.EffectiveSecurity(sys.ID)); !belt.IsZero() {
		if tr, ok := s.Transforms.Get(belt); ok {
			if c, ok := s.Celestials.Get(belt); ok {
				return tr.Pos, c.Radius
			}
		}
	}
	pos, _, _ := patrolAnchor(sys)
	return pos, trafficPatrolScatter
}

// warObserveKill feeds one death into the war: a Works hull destroyed at a
// front is charter attrition (credited like delivered materiel), and any
// death involving a Corporate Fleets hull is a witnessed skirmish.
func (s *Sim) warObserveKill(e, src ecs.Entity, def *content.NpcDef) {
	if s.war == nil || def == nil {
		return
	}
	side := s.factionOf(e)
	if side == factionOldWorks {
		s.warCredit(s.systemID, killAttrition*supplyPerFlip)
		if src == s.player {
			s.emit(Event{Kind: EvWar, Phase: WarKill, Severity: SevGood, Actor: src,
				Target: e, System: s.systemID,
				Text: "Works " + def.Name + " destroyed at the front"})
		}
	}
	if side == factionFleets || s.factionOf(src) == factionFleets {
		s.emit(Event{Kind: EvWar, Phase: WarSkirmish, Severity: SevInfo, Actor: e,
			Target: src, System: s.systemID,
			Text: "Front skirmish — " + s.nameOf(e) + " destroyed"})
	}
}

// ---------------------------------------------------------------------------
// Procurement (the sink)
// ---------------------------------------------------------------------------

// warRefreshBids replaces every standing war bid: last beat's lines are
// withdrawn and re-priced, so the book never accumulates stale war demand, and
// the fresh book's total exposure is budgeted against the treasury (a fill can
// therefore never overdraw it).
func (s *Sim) warRefreshBids() {
	for _, st := range s.stationsSorted() {
		s.warClearBook(st)
	}

	materiel := warMateriel(s.Content)
	budget := s.war.Treasury
	for _, st := range s.stationsSorted() {
		sys, ok := s.stationSystem(st)
		if !ok || s.war.Control[sys] != controlCharter {
			continue // works-held remnant books carry no war demand
		}
		book := s.market[st]
		for _, item := range materiel {
			price := warBidPrice(s, book, item)
			if price <= 0 {
				continue
			}
			qty := warBidQty
			if _, isModule := s.Content.Module(content.ModuleID(item)); isModule {
				qty = warBidModuleQty
			}
			if cost := price * float64(qty); cost > budget {
				qty = int(budget / price)
			}
			if qty <= 0 {
				continue
			}
			res, ok := s.PlaceOrder(st, OrderBuy, item, qty, price, OwnerWar)
			if !ok {
				continue
			}
			budget -= price * float64(res.Remaining)
		}
	}
}

// warClearBook withdraws every war bid resting on one station's book.
func (s *Sim) warClearBook(station StationID) {
	book := s.market[station]
	if book == nil {
		return
	}
	var ids []uint64
	for _, o := range book.Orders {
		if o.Owner == OwnerWar {
			ids = append(ids, o.ID)
		}
	}
	for _, id := range ids {
		s.CancelOrder(station, id, OwnerWar)
	}
}

// warClearBids withdraws the war demand of one system's stations — a system
// that just fell stops buying the materiel it now needs for itself.
func (s *Sim) warClearBids(sys content.SystemID) {
	for _, st := range s.stationsSorted() {
		if owner, ok := s.stationSystem(st); ok && owner == sys {
			s.warClearBook(st)
		}
	}
}

// warBidPrice is the war's reservation price for one line: one tick under the
// best ask, so a fresh bid never eats the station's own supply (a bid at or
// above the ask crosses it), or one tick above the displayed price when no ask
// rests, so the war is the best bid on that book. Zero when the item carries
// no base price at all.
func warBidPrice(s *Sim, book *MarketBook, item content.ItemID) float64 {
	if book != nil {
		if ask, ok := book.bestAsk(item); ok {
			return tickDown(ask.Price)
		}
	}
	p := s.marketPrice(book, item) // last → best bid → base
	if p <= 0 {
		return 0
	}
	return quantizeTick(p + TickStep(p))
}

// warMateriel is the derived materiel set: every module of a war kind (all
// weapons, the EWAR modules and the full tank rack — a mining laser and an
// afterburner are not war goods) plus every ammo line and charge, in ascending
// id order. The exact list is pinned in sim/war_test.go.
func warMateriel(c *content.Content) []content.ItemID {
	var out []content.ItemID
	for id, def := range c.Modules {
		if warKind(def.Kind) {
			out = append(out, content.ItemID(id))
		}
	}
	for id, def := range c.Items {
		if def.Category == content.CatAmmo || def.Category == content.CatCharge {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// warKind is the war-goods allow-list of module kinds: everything that shoots,
// jams or tanks. An unknown kind is not bought, so a new kind is a deliberate
// decision rather than a silent sink.
func warKind(kind string) bool {
	switch kind {
	case content.KindWeapon, content.KindStasisWeb, content.KindWarpScram,
		content.KindShieldBoost, content.KindShieldExtender,
		content.KindArmorRep, content.KindArmorPlate, content.KindResistCoat,
		content.KindCapBooster, content.KindCapRecharger:
		return true
	}
	return false
}

// warFill books one war-bid execution: the treasury pays for the goods (a
// transfer — the war mints nothing) and the value accrues as supply to the
// nearest front. Called from settle, the one point every execution passes.
func (s *Sim) warFill(station StationID, value float64) {
	if s.war == nil || value <= 0 {
		return
	}
	// shortcut: bids are budgeted under the treasury at post time, so a
	// shortfall here is a bug, not a state; clamp rather than go negative.
	s.war.Treasury = math.Max(0, s.war.Treasury-value)
	if front, ok := s.warNearestFront(station); ok {
		s.warCredit(front, value)
	}
}

// warNearestFront is the front a station's materiel is hauled to: the station's
// own system when it is on the front, else the nearest front by starmap
// distance (ties by id).
func (s *Sim) warNearestFront(station StationID) (content.SystemID, bool) {
	if s.war == nil || len(s.war.Contest) == 0 {
		return "", false
	}
	sysID, ok := s.stationSystem(station)
	if !ok {
		return "", false
	}
	if _, onFront := s.war.Contest[sysID]; onFront {
		return sysID, true
	}
	here := s.Content.Systems[sysID]
	if here == nil {
		return "", false
	}
	best, bestD := content.SystemID(""), math.MaxFloat64
	for _, id := range s.warContestIDs() {
		other := s.Content.Systems[id]
		if other == nil {
			continue
		}
		if d := here.Position.Dist(other.Position); d < bestD {
			best, bestD = id, d
		}
	}
	return best, best != ""
}

// ---------------------------------------------------------------------------
// Fee cut, remnant service mask
// ---------------------------------------------------------------------------

// routeFee burns a broker fee, sales tax or maintenance charge, routing the
// war's cut of it into the treasury first — money that used to vanish now
// feeds the war. The rest is burned exactly as before.
func (s *Sim) routeFee(amount float64) {
	if s.war == nil || amount <= 0 {
		return
	}
	s.war.Treasury += feeCut * amount
}

// remnant reports whether a station's system has fallen to the Works.
func (s *Sim) remnant(station StationID) bool {
	if s.war == nil || station == "" {
		return false
	}
	sys, ok := s.stationSystem(station)
	return ok && s.war.Control[sys] == controlWorks
}

// stationOffers reports whether a station still offers svc: the authored set,
// minus the charter services a remnant loses. Market, repair and refinery stay
// live (lawless); industry and fitting go dark, and the contract desk goes dark
// in execAcceptMission — no new charter work until a reclaim.
func (s *Sim) stationOffers(station StationID, svc string) bool {
	if !s.remnant(station) {
		return true
	}
	switch svc {
	case content.SvcIndustry, content.SvcFitting:
		return false
	}
	return true
}
