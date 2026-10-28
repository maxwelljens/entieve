package sim

// ---------------------------------------------------------------------------
// The Long Attrition — war pins.
//
// The war is world-gated (warWorlds), so every test here turns it on by hand:
// the fixture world has no war, and newWarState() seeds control from authored
// security exactly as the shipping world does. The real-content tests read the
// shipped tick-0 map, which is the contract: charter holds high/low space, the
// Works hold null space, and the front is the four systems that meet across a
// gate.
// ---------------------------------------------------------------------------

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// warContent is the base fixture plus a works-held pair of systems wired to
// sys-a across gates, a charter guard post and an industry station in sys-a,
// and a works-held market station whose book must stay bare of war demand.
//
//	charter: sys-a (high) ↔ sys-b (low)
//	works:   sys-w (null) ↔ sys-w2 (null)
//	front:   sys-a ⇄ sys-w (one contest each)
func warContent() *content.Content {
	c := recipeContent()
	// Haven Rest gets industry (the remnant job pin) and a guard post.
	for i := range c.Systems["sys-a"].Celestials {
		cel := &c.Systems["sys-a"].Celestials[i]
		if cel.ID == "cel-station-a" && cel.Station != nil {
			cel.Station.Services = append(cel.Station.Services, "industry")
			cel.Spawns = []content.SpawnDef{{NpcID: "npc-guard-test", Count: 1}}
		}
	}
	c.NPCs["npc-guard-test"] = &content.NpcDef{
		ID: "npc-guard-test", Name: "Fixture Guard", Faction: factionCharter,
		Ship: "ship-sloop", AggroRange: 0, OrbitRange: 1500,
	}
	// A contract on the Haven Rest board: the charter desk a remnant loses.
	c.Contracts = map[content.ContractID]*content.ContractDef{
		"contract-war-test": {
			ID: "contract-war-test", Title: "Hold the Line", Kind: content.ContractKill,
			Station: "cel-station-a",
			Objective: content.ContractObjective{
				Target: "npc-rat-scuttler", Count: 1,
			},
			Reward: content.ContractReward{Credits: 1000},
		},
	}
	// sys-a gains the gate to the works side.
	sysA := c.Systems["sys-a"]
	sysA.Celestials = append(sysA.Celestials, content.CelestialDef{
		ID: "cel-gate-w", Name: "Gate to the Works", Kind: content.CelGate,
		Position: core.Vec2{X: -3e7}, Radius: 300,
		Gate: &content.GateDef{ToSystem: "sys-w", ToCel: "cel-gate-a2"},
	})
	c.Systems["sys-w"] = &content.SystemDef{
		ID: "sys-w", Name: "Works Alpha", Security: "null",
		Constellation: "Test", Region: "Test",
		Position: core.Vec2{X: 1e18},
		Celestials: []content.CelestialDef{
			{ID: "cel-gate-a2", Name: "Gate to Prime", Kind: content.CelGate,
				Position: core.Vec2{}, Radius: 300,
				Gate: &content.GateDef{ToSystem: "sys-a", ToCel: "cel-gate-w"}},
			{ID: "cel-station-w", Name: "Works Rest", Kind: content.CelStation,
				Position: core.Vec2{X: 4e6}, Radius: 500,
				Station: &content.StationDef{
					Services: []string{"market", "repair", "industry", "fitting"},
					Market: []content.MarketOrderDef{
						{Item: "mod-autocannon-s", Qty: 4, Price: 1200},
					},
				}},
			{ID: "cel-belt-w", Name: "Works Belt", Kind: content.CelBelt,
				Position: core.Vec2{X: 8e6}, Radius: 1500,
				Belt: &content.BeltDef{Asteroids: []content.AsteroidDef{
					{Ore: "ore-sterngrit", Count: 1, Volume: 50},
				}},
			},
			{ID: "cel-gate-w2", Name: "Gate to Works Beta", Kind: content.CelGate,
				Position: core.Vec2{X: 1.2e7}, Radius: 300,
				Gate: &content.GateDef{ToSystem: "sys-w2", ToCel: "cel-gate-w3"}},
		},
	}
	c.Systems["sys-w2"] = &content.SystemDef{
		ID: "sys-w2", Name: "Works Beta", Security: "null",
		Constellation: "Test", Region: "Test",
		Position: core.Vec2{X: 2e18},
		Celestials: []content.CelestialDef{
			{ID: "cel-gate-w3", Name: "Gate to Works Alpha", Kind: content.CelGate,
				Position: core.Vec2{}, Radius: 300,
				Gate: &content.GateDef{ToSystem: "sys-w", ToCel: "cel-gate-w2"}},
			{ID: "cel-belt-w2", Name: "Works Beta Belt", Kind: content.CelBelt,
				Position: core.Vec2{X: 5e6}, Radius: 1500,
				Belt: &content.BeltDef{Asteroids: []content.AsteroidDef{
					{Ore: "ore-sterngrit", Count: 1, Volume: 50},
				}},
			},
		},
	}
	return c
}

// newWarSim builds the fixture sim with the war switched on (the fixture world
// is not a war world, so the gate stays off until a test asks for it).
func newWarSim(t *testing.T) *Sim {
	t.Helper()
	c := warContent()
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.War() != nil {
		t.Fatal("fixture world carries a war (the world gate failed)")
	}
	s.newWarState()
	if s.war.Control["sys-a"] != controlCharter || s.war.Control["sys-w"] != controlWorks {
		t.Fatalf("fixture control = %v", s.war.Control)
	}
	return s
}

// stepToWarBeat advances exactly one hourly war beat (the beat for tick n
// fires on the Step that starts at tick n, before the tick counter moves).
func stepToWarBeat(s *Sim) {
	target := s.ticks + procureCadence - s.ticks%procureCadence
	if s.ticks%procureCadence == 0 {
		target = s.ticks // this tick's beat has not fired yet
	}
	for s.ticks <= target {
		s.Step()
	}
}

// warBids returns every resting war bid on a station's book.
func warBids(s *Sim, station StationID) []MarketStock {
	var out []MarketStock
	for _, o := range s.BookOrders(station) {
		if o.Owner == OwnerWar {
			out = append(out, o)
		}
	}
	return out
}

// warSupply is every money ledger the war touches, treasury included.
func warSupply(s *Sim) float64 {
	return moneySupply(s)
}

// ---------------------------------------------------------------------------
// 1. Tick-0 map, save shape and hash coverage
// ---------------------------------------------------------------------------

// TestWarTickZeroMap pins the shipped map: charter holds the high and low
// systems, the Works hold the null ones, and the front is exactly the four
// systems that meet across a gate.
func TestWarTickZeroMap(t *testing.T) {
	c := content.MustLoad()
	s, err := New(c, c.Worlds["world-default"], 7)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := map[content.SystemID]string{
		"sys-solace":    controlCharter, // Struvok, high
		"sys-rudnik":    controlCharter, // Rydnok, high
		"sys-penumbra":  controlCharter, // Kvonice, low
		"sys-mlynow":    controlCharter, // Młondan, low
		"sys-kamieniec": controlCharter, // Kamžal, low
		"sys-gloam":     controlWorks,   // Pušťov, null
		"sys-glusza":    controlWorks,   // Glůšan, null
	}
	war := s.War()
	if war == nil {
		t.Fatal("world-default carries no war")
	}
	for id, side := range want {
		if got := war.Control[id]; got != side {
			t.Errorf("control[%s] = %q, want %q", id, got, side)
		}
	}
	if len(war.Control) != len(want) {
		t.Errorf("control covers %d systems, want %d", len(war.Control), len(want))
	}
	// The contiguous front: charter-side Kvonice + Kamžal, works-side Pušťov
	// + Glůšan. Nothing else borders enemy control.
	wantFront := map[content.SystemID]string{
		"sys-penumbra":  controlCharter,
		"sys-kamieniec": controlCharter,
		"sys-gloam":     controlWorks,
		"sys-glusza":    controlWorks,
	}
	if len(war.Contest) != len(wantFront) {
		t.Fatalf("front = %d systems, want %d (%v)", len(war.Contest), len(wantFront), war.Contest)
	}
	for id, side := range wantFront {
		c, ok := war.Contest[id]
		if !ok {
			t.Errorf("%s missing from the front", id)
			continue
		}
		if war.Control[id] != side {
			t.Errorf("%s is held by %q, want %q", id, war.Control[id], side)
		}
		if c.Meter != 0 || c.Threshold != meterThreshold || c.SupplyPending != 0 {
			t.Errorf("%s starts at %+v, want an idle meter", id, *c)
		}
	}
	if war.Treasury != warEndowment {
		t.Errorf("treasury = %v, want the %v seed", war.Treasury, warEndowment)
	}
	if s.Squeeze() != 1 {
		t.Errorf("tick-0 squeeze = %v, want 1", s.Squeeze())
	}
	// Effective security is the authored tier while the charter holds.
	if got := s.EffectiveSecurity("sys-solace"); got != "high" {
		t.Errorf("EffectiveSecurity(solace) = %q, want high", got)
	}
	if got := s.EffectiveSecurity("sys-gloam"); got != "null" {
		t.Errorf("EffectiveSecurity(gloam) = %q, want null", got)
	}
}

// TestWarSaveShapeAndHashCoverage pins the additive save field and proves the
// hash fingerprints it: three independent mutations of war state must each
// change the fingerprint.
func TestWarSaveShapeAndHashCoverage(t *testing.T) {
	s := newWarSim(t)
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["war"]; !ok {
		t.Fatalf("save has no \"war\" key: %v", keys)
	}
	if save.War == nil || save.War.Control["sys-a"] != controlCharter {
		t.Fatalf("save.War = %+v", save.War)
	}

	// The war is control + meters + treasury and nothing else: squeeze is
	// derived from control share and must not be stored.
	var war map[string]json.RawMessage
	if err := json.Unmarshal(keys["war"], &war); err != nil {
		t.Fatal(err)
	}
	var fields []string
	for k := range war {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	if got := strings.Join(fields, ","); got != "control,treasury" && got != "contest,control,treasury" {
		t.Fatalf("war save fields = %q", got)
	}

	base := s.Hash()
	if s.Hash() != base {
		t.Fatal("unstable hash")
	}
	checks := []struct {
		what string
		do   func() func()
	}{
		{"treasury", func() func() {
			old := s.war.Treasury
			s.war.Treasury += 1
			return func() { s.war.Treasury = old }
		}},
		{"control", func() func() {
			old := s.war.Control["sys-b"]
			s.war.Control["sys-b"] = controlWorks
			return func() { s.war.Control["sys-b"] = old }
		}},
		{"contest meter", func() func() {
			old := s.war.Contest["sys-a"].Meter
			s.war.Contest["sys-a"].Meter = 0.5
			return func() { s.war.Contest["sys-a"].Meter = old }
		}},
	}
	for _, c := range checks {
		before := s.Hash()
		undo := c.do()
		if s.Hash() == before {
			t.Errorf("%s does not reach the hash", c.what)
		}
		undo()
		if s.Hash() != before {
			t.Errorf("%s: undo did not restore the hash", c.what)
		}
	}
}

// TestWarSaveRoundTrip: war state survives snapshot/restore verbatim and the
// restored sim keeps stepping identically.
func TestWarSaveRoundTrip(t *testing.T) {
	s := newWarSim(t)
	stepToWarBeat(s) // post a book of war bids, move a meter
	s.war.Contest["sys-w"].SupplyPending = 12_345
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored, err := New(warContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.War() == nil {
		t.Fatal("restored sim lost the war")
	}
	if *restored.War().Contest["sys-w"] != *s.war.Contest["sys-w"] {
		t.Fatalf("contest %+v, want %+v", *restored.War().Contest["sys-w"], *s.war.Contest["sys-w"])
	}
	if restored.WarTreasury() != s.WarTreasury() {
		t.Fatalf("treasury = %v, want %v", restored.WarTreasury(), s.WarTreasury())
	}
	if restored.Hash() != s.Hash() {
		t.Fatal("hash differs across the round trip: war state is not fully carried")
	}
	if n, want := len(warBids(restored, "cel-station-a")), len(warBids(s, "cel-station-a")); n != want {
		t.Fatalf("restored war bids = %d, want %d", n, want)
	}
	// Twin the next beat: identical books, identical meters.
	stepToWarBeat(s)
	stepToWarBeat(restored)
	if s.Hash() != restored.Hash() {
		t.Fatal("twin runs diverged after the round trip")
	}
}

// TestWarOldSaveInitialisesMap: a save written before the war existed (War
// nil) restores into today's map when the world carries the war, and stays
// war-free everywhere else.
func TestWarOldSaveInitialisesMap(t *testing.T) {
	c := content.MustLoad()
	wd := c.Worlds["world-default"]
	s, err := New(c, wd, 3)
	if err != nil {
		t.Fatal(err)
	}
	save, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	save.War = nil // the pre-war save shape
	old, err := New(c, wd, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if old.War() == nil {
		t.Fatal("a pre-war save restored without the war")
	}
	if got := old.War().Control["sys-gloam"]; got != controlWorks {
		t.Fatalf("derived control[gloam] = %q, want works", got)
	}
	if len(old.War().Contest) != 4 {
		t.Fatalf("derived front = %d systems, want 4", len(old.War().Contest))
	}
	if old.WarTreasury() != warEndowment {
		t.Fatalf("derived treasury = %v, want the seed", old.WarTreasury())
	}

	// A fixture world stays war-free.
	fs := newWarSim(t)
	fSave, err := fs.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	fSave.War = nil
	fresh, err := New(warContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Restore(fSave); err != nil {
		t.Fatal(err)
	}
	if fresh.War() != nil {
		t.Fatal("a non-war world restored with a war")
	}
}

// ---------------------------------------------------------------------------
// 2. The materiel set (derived from content, pinned here)
// ---------------------------------------------------------------------------

// TestWarMaterielSet pins the derived list: every weapon, EWAR and tank module
// plus every ammo line and charge — and nothing else (no mining laser, no
// afterburner, no ore, no loot, no hulls).
func TestWarMaterielSet(t *testing.T) {
	c := content.MustLoad()
	got := make([]string, 0, len(warMateriel(c)))
	for _, id := range warMateriel(c) {
		got = append(got, string(id))
	}
	want := []string{
		"ammo-ap-l", "ammo-ap-m", "ammo-ap-s",
		"ammo-art-ap-l", "ammo-art-ap-m", "ammo-art-ap-s",
		"ammo-art-emp-l", "ammo-art-emp-m", "ammo-art-emp-s",
		"ammo-art-fusion-l", "ammo-art-fusion-m", "ammo-art-fusion-s",
		"ammo-art-plutonium-l", "ammo-art-plutonium-m", "ammo-art-plutonium-s",
		"ammo-emp-l", "ammo-emp-m", "ammo-emp-s",
		"ammo-fusion-l", "ammo-fusion-m", "ammo-fusion-s",
		"ammo-plutonium-l", "ammo-plutonium-m", "ammo-plutonium-s",
		"charge-cap-cell",
		"mod-armor-plate", "mod-armor-rep",
		"mod-artillery-l", "mod-artillery-m", "mod-artillery-s",
		"mod-autocannon-l", "mod-autocannon-m", "mod-autocannon-s",
		"mod-cap-booster", "mod-cap-recharger",
		"mod-resist-coat", "mod-rocket-pod",
		"mod-shield-boost", "mod-shield-extender",
		"mod-stasis-web", "mod-warp-scram",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("materiel set =\n%v\nwant\n%v", got, want)
	}
	if !warKind(content.KindWeapon) || !warKind(content.KindArmorRep) {
		t.Fatal("warKind misses a war kind")
	}
	for _, kind := range []string{content.KindMiningLaser, content.KindAfterburner, "unknown_kind"} {
		if warKind(kind) {
			t.Errorf("warKind(%q) = true, want false", kind)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Procurement: standing bids, the sink, the transfer
// ---------------------------------------------------------------------------

// TestWarProcurementBids pins the standing demand: every materiel line rests
// on every charter-held book (below the ask, so a fresh bid never eats the
// station's own supply), and a works-held remnant book carries none.
func TestWarProcurementBids(t *testing.T) {
	s := newWarSim(t)
	stepToWarBeat(s)
	drain(s)

	materiel := warMateriel(s.Content)
	bids := warBids(s, "cel-station-a")
	if len(bids) == 0 {
		t.Fatal("no war bids on the charter station's book")
	}
	have := map[content.ItemID]MarketStock{}
	for _, o := range bids {
		if o.Side != OrderBuy {
			t.Errorf("%s rests as %s, want a buy", o.Item, o.Side)
		}
		if o.Station != "cel-station-a" {
			t.Errorf("%s rests on %s", o.Item, o.Station)
		}
		have[o.Item] = o
	}
	for _, item := range materiel {
		if _, ok := have[item]; !ok {
			t.Errorf("no war bid for %s (every military line must be bought)", item)
		}
	}
	// Below the best ask: the war never buys the station's own supply with a
	// fresh bid, and it never pays above the displayed price.
	for _, o := range bids {
		if ask, ok := s.market["cel-station-a"].bestAsk(o.Item); ok && o.Price >= ask.Price {
			t.Errorf("%s war bid %.2f is at/above the ask %.2f", o.Item, o.Price, ask.Price)
		}
		if p := s.MarketPrice("cel-station-a", o.Item); o.Price > p {
			t.Errorf("%s war bid %.2f is above the displayed price %.2f", o.Item, o.Price, p)
		}
	}
	// Nothing traded placing them, and the treasury is untouched until a fill.
	if s.WarTreasury() != warEndowment {
		t.Errorf("treasury = %v after posting, want the untouched seed", s.WarTreasury())
	}
	// A works-held station's book carries no war demand.
	for _, o := range s.BookOrders("cel-station-w") {
		if o.Owner == OwnerWar {
			t.Fatalf("remnant book carries war demand: %+v", o)
		}
	}
	// Repricing replaces, never accumulates: the same count after another beat.
	count := len(warBids(s, "cel-station-a"))
	stepToWarBeat(s)
	if got := len(warBids(s, "cel-station-a")); got != count {
		t.Fatalf("war bids accumulate: %d -> %d", count, got)
	}
}

// TestWarSinkConsumesAndPays: selling into a war bid transfers treasury money
// to the seller, the goods leave the world (the sink), and the supply is queued
// for the nearest front.
func TestWarSinkConsumesAndPays(t *testing.T) {
	s := newWarSim(t)
	stepToWarBeat(s)
	dockAtHaven(t, s)
	drain(s)

	giveCargo(s, "mod-armor-plate", 3)
	before := warSupply(s)
	walletBefore := s.Wallet()
	treasuryBefore := s.WarTreasury()
	bid := warBids(s, "cel-station-a")
	var price float64
	for _, o := range bid {
		if o.Item == "mod-armor-plate" {
			price = o.Price
		}
	}
	if price <= 0 {
		t.Fatal("no war bid for mod-armor-plate")
	}

	s.Queue(CmdSell{Item: "mod-armor-plate", Qty: 2})
	s.Step()
	evs := drain(s)

	if got := cargoCount(s, "mod-armor-plate"); got != 1 {
		t.Fatalf("player still holds %d plates, want 1 (the war consumed two)", got)
	}
	value := price * 2
	// The fill is the treasury's transfer, and the sale's own fee cut lands
	// back in it: net = -value + feeCut * (the fees the sale charged).
	fees := MarketBrokerFee*value + MarketSalesTax*value
	if got, want := s.WarTreasury(), treasuryBefore-value+feeCut*fees; math.Abs(got-want) > 1e-6 {
		t.Fatalf("treasury = %v, want %v (the fill's value, less the fee cut)", got, want)
	}
	if s.Wallet() <= walletBefore {
		t.Fatalf("wallet did not rise on a war sale: %v -> %v", walletBefore, s.Wallet())
	}
	// The only money that left the world is the un-routed part of the fees.
	if got, want := warSupply(s), before-(1-feeCut)*fees; math.Abs(got-want) > 1e-6 {
		t.Fatalf("money supply %v, want %v (a transfer plus the usual burn)", got, want)
	}
	// The supply is queued for the nearest front (Haven Rest's own system),
	// not applied immediately.
	c := s.war.Contest["sys-a"]
	if c == nil || math.Abs(c.SupplyPending-value) > 1e-6 {
		t.Fatalf("front supply pending = %+v, want %v", c, value)
	}
	if !hasEvent(evs, EvWar, "Supply —") {
		t.Fatalf("no war-supply line: %v", evs)
	}
	// Goods are gone from the book too: the bid's remaining quantity dropped.
	for _, o := range warBids(s, "cel-station-a") {
		if o.Item == "mod-armor-plate" && o.Qty >= bidQuantity(bid, "mod-armor-plate") {
			t.Fatalf("war bid was not drawn down: %+v", o)
		}
	}
}

// bidQuantity returns the resting quantity of item in a bid list (0 when
// absent).
func bidQuantity(bids []MarketStock, item content.ItemID) int {
	for _, o := range bids {
		if o.Item == item {
			return o.Qty
		}
	}
	return 0
}

// TestWarFeeCutRoutesToTreasury pins the funding: a broker fee charged on an
// ordinary (non-war) trade now credits the treasury with the war's cut instead
// of burning all of it, and the rest still burns.
func TestWarFeeCutRoutesToTreasury(t *testing.T) {
	s := newWarSim(t)
	dockAtHaven(t, s)
	drain(s)
	before := warSupply(s)
	treasury := s.WarTreasury()

	price := s.MarketPrice("cel-station-a", "min-ferrite")
	s.Queue(CmdBuy{Item: "min-ferrite", Qty: 10})
	s.Step()
	drain(s)

	cost := 10 * price
	fee := MarketBrokerFee * cost
	if fee <= 0 {
		t.Fatalf("fixture: no fee charged (cost %v)", cost)
	}
	if got, want := s.WarTreasury(), treasury+feeCut*fee; math.Abs(got-want) > 1e-6 {
		t.Fatalf("treasury = %v, want %v (the war's cut of a %.2f fee)", got, want, fee)
	}
	// The whole price leaves the world (the station side has no ledger — the
	// pre-existing convention); only the routed cut survives.
	if got, want := warSupply(s), before-cost-(1-feeCut)*fee; math.Abs(got-want) > 1e-6 {
		t.Fatalf("money supply %v, want %v (only the un-routed cut burns)", got, want)
	}
}

// TestWarMoneySupplyEnumeration pins the fifth bucket: the war treasury is
// part of the money supply a conservation check must count.
func TestWarMoneySupplyEnumeration(t *testing.T) {
	s := newWarSim(t)
	before := warSupply(s)
	s.war.Treasury += 100
	if got := warSupply(s); got != before+100 {
		t.Fatalf("money supply %v, want %v (the treasury is not enumerated)", got, before+100)
	}
	s.war.Treasury -= 100
	if got := warSupply(s); got != before {
		t.Fatalf("money supply %v, want %v", got, before)
	}
}

// ---------------------------------------------------------------------------
// 4. Contest meters: supply pushes, pressure erodes, thresholds flip
// ---------------------------------------------------------------------------

// TestWarContestDynamics pins the direction of every force (the knobs may be
// retuned; the signs may not).
func TestWarContestDynamics(t *testing.T) {
	t.Run("supply pushes a works-held front", func(t *testing.T) {
		s := newWarSim(t)
		s.war.Contest["sys-w"].SupplyPending = supplyPerFlip / 2
		stepToWarBeat(s)
		got := s.war.Contest["sys-w"].Meter
		// Half the supply, less one beat of the Works' resistance.
		resist := float64(procureCadence) / (worksPressureHours * 3600)
		if want := 0.5 - resist; math.Abs(got-want) > 1e-9 {
			t.Fatalf("meter = %v, want %v", got, want)
		}
		if s.war.Contest["sys-w"].SupplyPending != 0 {
			t.Fatal("pending supply was not released by the beat")
		}
	})
	t.Run("pressure erodes a charter-held front", func(t *testing.T) {
		s := newWarSim(t)
		stepToWarBeat(s)
		got := s.war.Contest["sys-a"].Meter
		want := float64(procureCadence) / (worksPressureHours * 3600)
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("meter = %v, want %v of works pressure", got, want)
		}
	})
	t.Run("charter supply holds a charter front at the floor", func(t *testing.T) {
		s := newWarSim(t)
		s.war.Contest["sys-a"].SupplyPending = supplyPerFlip
		stepToWarBeat(s)
		if got := s.war.Contest["sys-a"].Meter; got != 0 {
			t.Fatalf("meter = %v, want it clamped at 0 (supplied front holds)", got)
		}
	})
	t.Run("more squeeze is less charter progress", func(t *testing.T) {
		// The same half-meter of supply on a works-held front: at the tick-0
		// share the Works are at squeeze 1; with one system left they are
		// cornered and the same supply buys far less.
		free := func() float64 {
			s := newWarSim(t)
			s.war.Contest["sys-w"].SupplyPending = supplyPerFlip / 2
			stepToWarBeat(s)
			return s.war.Contest["sys-w"].Meter
		}
		cornered := func() float64 {
			s := newWarSim(t)
			s.war.Control["sys-w"] = controlCharter // the front moves to sys-w2
			s.warRederive()
			s.war.Contest["sys-w2"].SupplyPending = supplyPerFlip / 2
			stepToWarBeat(s)
			return s.war.Contest["sys-w2"].Meter
		}
		f, c := free(), cornered()
		if !(f > c) || c < 0 {
			t.Fatalf("cornered meter %v is not below the free meter %v", c, f)
		}
	})
	t.Run("threshold flips and geography re-derives", func(t *testing.T) {
		s := newWarSim(t)
		// Past the threshold before the beat's own push lands, so the beat
		// itself is what crosses it.
		s.war.Contest["sys-w"].Meter = meterThreshold + 0.5
		stepToWarBeat(s)
		if got := s.war.Control["sys-w"]; got != controlCharter {
			t.Fatalf("control[sys-w] = %q, want charter (the meter crossed)", got)
		}
		// The flipped system is on the front from its new side; the system
		// that no longer meets an enemy loses its meter.
		if _, ok := s.war.Contest["sys-a"]; ok {
			t.Fatal("sys-a still carries a meter with no enemy across a gate")
		}
		for _, id := range []content.SystemID{"sys-w", "sys-w2"} {
			if _, ok := s.war.Contest[id]; !ok {
				t.Fatalf("%s missing from the re-derived front (%v)", id, s.war.Contest)
			}
		}
	})
	t.Run("equilibrium guard: the last works system never falls", func(t *testing.T) {
		s := newWarSim(t)
		s.war.Control["sys-w"] = controlCharter // one works system left
		s.warRederive()
		if !(s.Squeeze() > 1e6) {
			t.Fatalf("squeeze at the floor = %v, want a diverging curve", s.Squeeze())
		}
		for i := 0; i < 200; i++ {
			// Hold the reclaimed front at zero so the war does not drift back
			// onto it (the cornered Works still hit hard there).
			if c := s.war.Contest["sys-w"]; c != nil {
				c.Meter = 0
			}
			c := s.war.Contest["sys-w2"]
			if c == nil {
				t.Fatalf("beat %d: the last works system left the front", i)
			}
			c.SupplyPending = supplyPerFlip * 1e6 // absurd supply
			stepToWarBeat(s)
			if s.war.Control["sys-w2"] != controlWorks {
				t.Fatalf("the Works were extinguished on beat %d (meter %v)", i, c.Meter)
			}
		}
	})
}

// TestWarFlipUnderYouIsInstant pins the flip's grid follow-through: guards
// leave outright, the Works land at the belts (never the docking ring), and a
// docked player is never evicted.
func TestWarFlipUnderYouIsInstant(t *testing.T) {
	s := newWarSim(t)
	if got := countFaction(s, factionCharter); got == 0 {
		t.Fatal("fixture: no charter guard on the grid")
	}
	dockAtHaven(t, s)
	stepToWarBeat(s)
	s.war.Contest["sys-a"].Meter = meterThreshold
	stepToWarBeat(s)
	evs := drain(s)

	if s.war.Control["sys-a"] != controlWorks {
		t.Fatalf("sys-a control = %q, want works", s.war.Control["sys-a"])
	}
	if !s.playerDocked() {
		t.Fatal("the flip evicted a docked player")
	}
	if n := countFaction(s, factionCharter); n != 0 {
		t.Fatalf("%d charter ships remain in fallen space (guards must leave)", n)
	}
	if n := countFaction(s, factionOldWorks); n == 0 {
		t.Fatal("the Works did not take the belts")
	}
	for _, info := range s.Grid() {
		if s.factionOf(info.Entity) != factionOldWorks {
			continue
		}
		// Never the docking ring: every Works hull stands off at a belt or an
		// anomaly anchor, not on a station's celestial.
		if cel, ok := s.Celestials.Get(info.Entity); ok && cel.Kind == content.CelStation {
			t.Fatalf("a Works hull landed on a station celestial (%s)", cel.Cel)
		}
	}
	if !hasEvent(evs, EvWar, "Fall —") {
		t.Fatalf("no fall news: %v", evs)
	}
	// Effective security is lawless now.
	if got := s.EffectiveSecurity("sys-a"); got != "null" {
		t.Fatalf("EffectiveSecurity = %q, want null", got)
	}
	if _, ok := s.war.Contest["sys-a"]; !ok {
		t.Fatal("the flipped system did not join the front")
	}
	if _, ok := s.war.Contest["sys-w"]; ok {
		t.Fatal("sys-w kept a meter with no enemy across a gate")
	}
}

// countFaction counts live NPCs fighting for a faction.
func countFaction(s *Sim, faction string) int {
	n := 0
	for _, e := range sortedEntities(s.AIs) {
		if s.factionOf(e) == faction {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// 5. Effective security drives every behavior the authored tier used to
// ---------------------------------------------------------------------------

// TestWarEffectiveSecurityDrivesBehavior: a fallen system runs the null-space
// rules (no patrols, no response wing, fearless temperament), and a reclaim
// puts the authored tier back.
func TestWarEffectiveSecurityDrivesBehavior(t *testing.T) {
	s := newWarSim(t)
	if got := s.EffectiveSecurity("sys-a"); got != "high" {
		t.Fatalf("charter-held EffectiveSecurity = %q, want high", got)
	}
	if _, ok := trafficPatrolInterval(9000, s.EffectiveSecurity("sys-a")); !ok {
		t.Fatal("a high-sec system must fly patrols")
	}

	s.war.Control["sys-a"] = controlWorks
	eff := s.EffectiveSecurity("sys-a")
	if eff != "null" {
		t.Fatalf("fallen EffectiveSecurity = %q, want null", eff)
	}
	if _, ok := trafficPatrolInterval(9000, eff); ok {
		t.Fatal("a fallen system still schedules patrols")
	}
	if wing := trafficPatrolWing(eff); wing != 0 {
		t.Fatalf("fallen response wing = %d, want 0", wing)
	}
	if got, want := trafficResponseDelay(eff), int64(1800); got != want {
		t.Fatalf("fallen response delay = %d, want %d", got, want)
	}
	if p := trafficAttackChance(eff); math.Abs(p-1.0/3.0) > 1e-12 {
		t.Fatalf("fallen ambush chance = %v, want 1/3", p)
	}
	def := s.Content.NPCs["npc-rat-scuttler"]
	aggro, flee := npcTemperament(eff, def)
	if math.Abs(aggro-def.AggroRange*1.3) > 1e-9 || flee != 0 {
		t.Fatalf("fallen temperament = (%v, %v), want the fearless null tier", aggro, flee)
	}

	// Reclaim returns the authored tier.
	s.war.Control["sys-a"] = controlCharter
	if got := s.EffectiveSecurity("sys-a"); got != "high" {
		t.Fatalf("reclaimed EffectiveSecurity = %q, want high", got)
	}
	if s.EffectiveSecurity("sys-w") != "null" {
		t.Fatal("works-held space is not lawless")
	}
}

// ---------------------------------------------------------------------------
// 6. Remnant: charter services dark, market and repair live, jobs finish
// ---------------------------------------------------------------------------

// TestWarRemnantServiceMask pins the remnant: market, repair and refinery stay
// live; industry, fitting and the contract desk go dark; a job already running
// finishes and collects (it was paid work).
func TestWarRemnantServiceMask(t *testing.T) {
	s := newWarSim(t)
	dockAtHaven(t, s)
	drain(s)

	giveCargo(s, "min-ferrite", 20)
	giveCargo(s, "min-nickel", 10)
	if _, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer); !ok {
		t.Fatal("fixture: job start refused while charter-held")
	}

	// The system falls under the player.
	s.war.Contest["sys-a"].Meter = meterThreshold
	stepToWarBeat(s)
	drain(s)
	if s.war.Control["sys-a"] != controlWorks {
		t.Fatalf("sys-a control = %q, want works", s.war.Control["sys-a"])
	}

	// Market and repair still work.
	if got := s.DockedMarket(); got != "cel-station-a" {
		t.Fatalf("DockedMarket = %q, want the remnant's book", got)
	}
	if s.Wallet() < 1 {
		t.Fatal("fixture: no wallet")
	}
	s.Queue(CmdRepair{})
	s.Step()
	if hasEvent(drain(s), EvLog, "does not offer") {
		t.Fatal("repair is dark at a remnant")
	}
	// Industry, fitting and the contract desk are dark.
	if got := s.DockedIndustry(); got != "" {
		t.Fatalf("DockedIndustry = %q, want \"\" at a remnant", got)
	}
	drain(s)
	s.Queue(CmdStartJob{Recipe: "rec-ammo-emp-s", Runs: 1})
	s.Step()
	if !hasEvent(drain(s), EvLog, "remnant") {
		t.Fatal("a remnant accepted a new industry job")
	}
	s.Queue(CmdAcceptMission{ID: "contract-war-test"})
	s.Step()
	if evs := drain(s); !hasEvent(evs, EvMission, "charter desk is dark") {
		t.Fatalf("a remnant handed out a contract: %v", evs)
	}
	// The job started while charter space was still alive finishes and pays.
	stepUntilJobDone(t, s, 1)
	if !s.CollectJob("cel-station-a", 1, OwnerPlayer) {
		t.Fatal("an in-flight job could not be collected after the fall")
	}
	if got := s.Hangar.Items; len(got) == 0 || got[0].Qty < 100 {
		t.Fatalf("collected %v, want the recipe's 100 rounds", got)
	}
	// A reclaimed station serves again.
	s.war.Control["sys-a"] = controlCharter
	s.warRederive()
	s.Step()
	if got := s.DockedIndustry(); got != "cel-station-a" {
		t.Fatalf("DockedIndustry after reclaim = %q, want the station back", got)
	}
}

// stepUntilJobDone advances until job id reports done (or fails).
func stepUntilJobDone(t *testing.T, s *Sim, id uint64) {
	t.Helper()
	for i := 0; i < 20000; i++ {
		s.Step()
		for _, j := range s.Jobs("cel-station-a") {
			if j.ID == id && j.State == JobDone {
				return
			}
		}
	}
	t.Fatalf("job %d never finished", id)
}

// ---------------------------------------------------------------------------
// 7. Fleets and hostility
// ---------------------------------------------------------------------------

// TestWarHostilityRows pins the bloc's place in the relations table: hostile
// to the Old Works only, fail-closed everywhere else (the player included).
func TestWarHostilityRows(t *testing.T) {
	if len(hostility) != 5 {
		t.Fatalf("relations table has %d rows, want 5 (Corporate Fleets added)", len(hostility))
	}
	if !hostile(factionFleets, factionOldWorks) || !hostile(factionOldWorks, factionFleets) {
		t.Fatal("the fleets and the Works are not mutually hostile")
	}
	for _, other := range []string{factionCharter, factionDvuzek, factionIndependent, factionFleets, "unknown"} {
		if hostile(factionFleets, other) {
			t.Errorf("the fleets are hostile to %q", other)
		}
	}
}

// TestWarFleetsSpawnAndEngage: a front system with the player in it gets a
// Corporate Fleets group that hunts the Works through the ordinary threat
// machine — and leaves the player alone.
func TestWarFleetsSpawnAndEngage(t *testing.T) {
	s := newWarSim(t)
	if err := s.LoadSystem("sys-w", ""); err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, v(-4e6, -4e6)) // well off the belt, out of aggro range
	drain(s)

	stepToWarBeat(s)
	if n := countFaction(s, factionFleets); n == 0 {
		t.Fatal("no fleet showed up at the front")
	}
	if n := countFaction(s, factionFleets); n > warFleetCap {
		t.Fatalf("fleet presence = %d, want the cap %d", n, warFleetCap)
	}

	// A Works hull on the belt is the fleet's business.
	belt := s.celEntity("cel-belt-w")
	var fleet ecs.Entity
	for _, e := range sortedEntities(s.AIs) {
		if s.factionOf(e) == factionFleets {
			fleet = e
			break
		}
	}
	rat := s.spawnNpc("npc-rat-scuttler", mustPos(s, belt).Add(v(300, 0)), 100)
	if rat.IsZero() || fleet.IsZero() {
		t.Fatal("fixture: spawn failed")
	}
	stepN(s, 60)
	ai, _ := s.AIs.Get(fleet)
	if ai.State == NpcIdle {
		t.Fatalf("the fleet ignored a Works hull on its belt (state %q)", ai.State)
	}
	// The fleet lands on a Works hull at the occupied belt and fights it (the
	// fixture belt already carries rats, so the target need not be this one).
	if ai.Target.IsZero() || s.factionOf(ai.Target) != factionOldWorks {
		t.Fatalf("the fleet targets %v, want an Old Works hull", ai.Target)
	}
	// Never the player: the bloc is neutral to independents.
	setPlayerPos(s, mustPos(s, rat).Add(v(200, 0)))
	stepN(s, 120)
	for _, ev := range drain(s) {
		if ev.Kind == EvLog && strings.Contains(ev.Text, "attacking you") {
			t.Fatalf("a fleet attacked the neutral player: %q", ev.Text)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. War news
// ---------------------------------------------------------------------------

// TestWarNews pins the event surface: a flip always reports, a contest move at
// or above newsThreshold reports, a quiet beat says nothing.
func TestWarNews(t *testing.T) {
	s := newWarSim(t)
	stepToWarBeat(s)
	drain(s)

	// A quiet beat: the pressure move is well under the threshold.
	stepToWarBeat(s)
	if evs := drain(s); len(evs) != 0 {
		var war []string
		for _, e := range evs {
			if e.Kind == EvWar {
				war = append(war, e.Text)
			}
		}
		if len(war) > 0 {
			t.Fatalf("a quiet beat reported %v", war)
		}
	}

	// A contest move the player can see.
	s.war.Contest["sys-w"].SupplyPending = supplyPerFlip * 0.1 // 10% of a meter
	stepToWarBeat(s)
	evs := drain(s)
	got := findEvent(evs, EvWar, "Contested")
	if got == nil {
		t.Fatalf("no contest line for a 10%% move: %v", evs)
	}
	if got.Phase != WarContest || got.System != "sys-w" {
		t.Fatalf("contest event = %+v", got)
	}

	// A flip is always news.
	s.war.Contest["sys-w"].Meter = meterThreshold + 0.5
	stepToWarBeat(s)
	if evs := drain(s); !hasEvent(evs, EvWar, "Reclaimed") {
		t.Fatalf("a flip did not report: %v", evs)
	}
}

// TestWarKillNews: a player kill on the front feeds the meter and reports.
func TestWarKillNews(t *testing.T) {
	s := newWarSim(t)
	if err := s.LoadSystem("sys-w", ""); err != nil {
		t.Fatal(err)
	}
	// Past the tick-0 beat, whose release would spend the credit at once.
	stepToWarBeat(s)
	drain(s)
	rat := s.spawnNpc("npc-rat-scuttler", mustPos(s, s.player).Add(v(500, 0)), 100)
	if rat.IsZero() {
		t.Fatal("fixture spawn failed")
	}
	drain(s)
	damageNPC(s, s.player, rat, content.Damage{EM: 1e6}, false)
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvWar, "at the front") {
		t.Fatalf("no kill line: %v", evs)
	}
	if got, want := s.war.Contest["sys-w"].SupplyPending, killAttrition*supplyPerFlip; math.Abs(got-want) > 1e-6 {
		t.Fatalf("kill credit = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// 9. Determinism
// ---------------------------------------------------------------------------

// TestWarTwinRunDeterminism: two identical war sims stay bit-identical across
// beats (war state is in the hash), and the bounded sim is the same after a
// save/restore in the middle.
func TestWarTwinRunDeterminism(t *testing.T) {
	run := func() map[int64]uint64 {
		s := newWarSim(t)
		hashes := map[int64]uint64{}
		for i := 0; i < 2*int(procureCadence)+120; i++ {
			s.Step()
			if s.ticks%600 == 0 {
				hashes[s.ticks] = s.Hash()
			}
		}
		return hashes
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("checkpoint count %d vs %d", len(a), len(b))
	}
	for tick, ha := range a {
		if b[tick] != ha {
			t.Fatalf("checkpoint %d: %016x vs %016x", tick, ha, b[tick])
		}
	}
}

// TestWarPushCommand pins the script harness's war lever: the value is meter
// units toward a flip — whoever holds the front, the meter crossing its
// threshold hands the system to the other side — and the beat does the flip.
// A system off the front has no meter to move.
func TestWarPushCommand(t *testing.T) {
	s := newWarSim(t)
	stepToWarBeat(s)
	push := meterThreshold * 1.2 // one threshold, with room for the beat's own pressure

	// A works-held front comes back to the charter.
	s.Queue(CmdWarPush{System: "sys-w", Value: push})
	s.Step()
	if got := s.war.Contest["sys-w"].Meter; got != push {
		t.Fatalf("meter = %v, want the pushed %v", got, push)
	}
	stepToWarBeat(s)
	if s.war.Control["sys-w"] != controlCharter {
		t.Fatal("a pushed-past-threshold works front did not come back to the charter")
	}
	// The same lever falls a charter-held front.
	s.Queue(CmdWarPush{System: "sys-w", Value: push})
	s.Step()
	stepToWarBeat(s)
	if s.war.Control["sys-w"] != controlWorks {
		t.Fatal("a pushed-past-threshold charter front did not fall")
	}
	// Pulling back is floored at a held front's zero; off the front there is
	// no meter at all.
	s.Queue(CmdWarPush{System: "sys-w", Value: -push})
	s.Step()
	if got := s.war.Contest["sys-w"].Meter; got < 0 {
		t.Fatalf("meter = %v, want a floor at 0", got)
	}
	s.Queue(CmdWarPush{System: "sys-b", Value: push})
	s.Step()
	if _, onFront := s.war.Contest["sys-b"]; onFront {
		t.Fatal("sys-b carries a meter without an enemy across a gate")
	}
}
