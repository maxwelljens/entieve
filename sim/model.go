// Package sim is Entieve's deterministic game simulation: components,
// commands, events and the systems that advance them.
//
// sim never imports Ebitengine. It is fully drivable headlessly: AI agents
// and tests issue Command values through Queue and read Event values back —
// this is the engine's controllability seam.
package sim

import (
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// TickRate is the fixed simulation rate in ticks per second.
const TickRate = 60

// DT is the simulated seconds advanced per Step.
const DT = 1.0 / 60.0

// ---------------------------------------------------------------------------
// Components
// ---------------------------------------------------------------------------

// Name is the display name of an entity.
type Name struct{ Value string }

// Transform is the grid position and heading of an entity.
type Transform struct {
	Pos    core.Vec2
	Facing float64 // radians, core.Vec2.Angle convention
}

// IntentKind enumerates movement intents.
type IntentKind string

const (
	IntentStop     IntentKind = "stop"
	IntentManual   IntentKind = "manual"
	IntentApproach IntentKind = "approach"
	IntentOrbit    IntentKind = "orbit"
	IntentAlign    IntentKind = "align"
)

// Intent is desired sublight movement.
type Intent struct {
	Kind      IntentKind
	Target    ecs.Entity // approach/orbit target; zero for none
	Range     float64    // orbit range, m
	Dir       core.Vec2  // manual direction (unit)
	SpeedFrac float64    // manual throttle 0..1
}

// Motion is the movement state of a mobile entity.
type Motion struct {
	Vel             core.Vec2
	Mass            float64 // kg
	MaxSpeed        float64 // m/s, base
	Agility         float64 // align factor; align time = agility * 10 s
	SigRadius       float64 // m
	SpeedMultiplier float64 // 1 = none; set by afterburner cycles
	WebFactor       float64 // multiplier applied while webbed (1 = none)
	WebbedUntil     float64 // sim time the last web expires
}

// EffectiveMaxSpeed returns speed with prop/web modifiers applied.
func (m *Motion) EffectiveMaxSpeed(now float64) float64 {
	sp := m.MaxSpeed * m.SpeedMultiplier
	if now < m.WebbedUntil {
		sp *= m.WebFactor
	}
	return sp
}

// ShipState is hull identity and the three EVE damage layers.
type ShipState struct {
	Hull                              content.ShipID
	Shield, Armor, Structure          float64 // current HP
	MaxShield, MaxArmor, MaxStructure float64
	Resists                           content.Resists // base hull + passive module bonuses
	ScrambledUntil                    float64         // warp disruption expiry (sim time)
}

// Capacitor is the EVE capacitor model (GJ).
type Capacitor struct {
	Current  float64
	Capacity float64
	Tau      float64 // recharge time constant, seconds
}

// Cargo is the item storage of an entity. Units are whole items; Volume()
// helpers live in cargo.go.
type Cargo struct {
	Stacks      []content.CargoStack // general hold
	Capacity    float64              // m³
	Ore         []content.CargoStack // dedicated ore hold
	OreCapacity float64              // m³; 0 means ore uses Stacks
	Remainder   float64              // fractional m³ of pending ore unit
}

// SlotRef addresses one module slot by class and index.
type SlotRef struct {
	Class string // content.SlotHigh | content.SlotMid | content.SlotLow
	Index int
}

// ModuleRuntime is the live state of one fitted module.
type ModuleRuntime struct {
	Def       content.ModuleID
	Active    bool
	CycleEnds float64 // sim time the current cycle completes (0 = idle)
	LastWarn  float64 // sim time of last "cannot activate" warning
}

// Fitting is a ship's module loadout. High/Mid/Low hold module IDs;
// Runtimes hold parallel live state.
type Fitting struct {
	High, Mid, Low       []content.ModuleID
	HighRT, MidRT, LowRT []ModuleRuntime
}

// Defs returns the module IDs of the given slot class.
func (f *Fitting) Defs(class string) []content.ModuleID {
	switch class {
	case content.SlotHigh:
		return f.High
	case content.SlotMid:
		return f.Mid
	case content.SlotLow:
		return f.Low
	}
	return nil
}

// Runtimes returns the live state of the given slot class.
func (f *Fitting) Runtimes(class string) []ModuleRuntime {
	switch class {
	case content.SlotHigh:
		return f.HighRT
	case content.SlotMid:
		return f.MidRT
	case content.SlotLow:
		return f.LowRT
	}
	return nil
}

// Lock is one target-lock entry.
type Lock struct {
	Target   ecs.Entity
	Progress float64 // 0..1; >= 1 means locked
}

// Targeting is locking state and lock capability.
type Targeting struct {
	Locks      []Lock
	MaxTargets int
	MaxRange   float64 // m
	ScanRes    float64
}

// Locked reports whether e is fully locked.
func (t *Targeting) Locked(e ecs.Entity) bool {
	for _, l := range t.Locks {
		if l.Target == e && l.Progress >= 1 {
			return true
		}
	}
	return false
}

// Pilot marks the player-controlled entity.
type Pilot struct{ Player bool }

// NpcAIState is the NPC behavior state machine state.
type NpcAIState string

const (
	NpcIdle     NpcAIState = "idle"
	NpcApproach NpcAIState = "approach"
	NpcOrbit    NpcAIState = "orbit"
	NpcFlee     NpcAIState = "flee"
	NpcReturn   NpcAIState = "return"
)

// NpcAI is the behavior state of an NPC ship.
type NpcAI struct {
	Npc    content.NpcID
	State  NpcAIState
	Target ecs.Entity // current hostile (usually the player)
	Home   core.Vec2  // spawn anchor
	Rand   *core.RNG  // per-NPC stream (Fork'd); marshaled via Snapshot/RestoreRNG
	// LastHitBy is the source of the most recent damage this ship took
	// (aggro trigger (b): only player-attributed damage aggroes an NPC onto
	// the player — third-party fire, e.g. a patrol during a convoy ambush,
	// must not). Persisted with the rest of the NPC state (SavedNpcAI).
	LastHitBy ecs.Entity
}

// Asteroid is a minable rock.
type Asteroid struct {
	Ore       content.ItemID
	Remaining float64 // m³ left
	// Belt and Index address the rock's persistent belt-state slot
	// ((system, belt celestial, def-order index)). The state mirrors
	// Remaining while the rock is on the grid, so depletion survives
	// save/restore and system reloads (Save.Belts).
	Belt  content.CelID
	Index int
}

// BeltRock is one rock's persistent belt state: the volume remaining
// against the definition volume it was seeded with (Volume is the
// regrowth cap, persisted so the state is self-contained).
type BeltRock struct {
	Ore       content.ItemID `json:"ore"`
	Remaining float64        `json:"remaining"`
	Volume    float64        `json:"volume"`
}

// BeltState is one belt's persistent state. Rocks are in BeltDef order
// (the flat rock index spawnCelestial iterates); GrownAt is the tick the
// regrowth window last advanced (see systemBelts).
type BeltState struct {
	System  content.SystemID `json:"system"`
	Cel     content.CelID    `json:"cel"`
	Rocks   []BeltRock       `json:"rocks,omitempty"`
	GrownAt int64            `json:"grown_at"`
}

// beltKey addresses one belt's persistent state universe-wide.
type beltKey struct {
	System content.SystemID
	Cel    content.CelID
}

// Celestial marks a static grid object.
type Celestial struct {
	Kind   string // content.Cel* kind
	Cel    content.CelID
	Radius float64
}

// Stargate is a jump connection to another system.
type Stargate struct {
	ToSystem content.SystemID
	ToCel    content.CelID
}

// Station offers services (content: "repair"|"market"|"refine"|"fitting").
type Station struct{ Services []string }

// Docked marks an entity inside a station (off-grid).
type Docked struct{ At content.CelID }

// WarpDrive is the warp state machine.
type WarpDrive struct {
	Phase string // "" | "align" | "warp" | "land"
	To    core.Vec2
	// Dist is the total warp distance (m) of the current warp; Speed is the
	// current along-vector speed (m/s) — presentation state for warp UI.
	// Dist tracks the live distance to the landing point while aligning and
	// freezes at the tunnel profile distance when the warp phase starts.
	Dist  float64
	Speed float64
}

// Wreck is the remains of a destroyed ship, holding rolled loot.
type Wreck struct {
	Loot      []content.CargoStack
	ExpiresAt float64
}

// Traffic phases (Traffic.Phase of a live convoy hauler).
const (
	// TrafficTransit is a hauler flying its one-way leg: warp out, retry on
	// interruption, deliver (station) or despawn (gate/belt) on arrival.
	TrafficTransit = "transit"
)

// Traffic outcome phases (TrafficAttack.Outcome).
const (
	TrafficOutcomeDestroyed = "destroyed"
	TrafficOutcomeEscaped   = "escaped"
	TrafficOutcomeSaved     = "saved"
)

// TrafficAttack is the live ambush state of one convoy hauler: the raiders
// engaged with it, when the hauler last may flee, when the patrol response
// is due (RespondAt < 0: never), and how the incident resolved ("" while
// running).
type TrafficAttack struct {
	Raiders   []ecs.Entity `json:"raiders"`
	Started   float64      `json:"started"`    // sim time of the ambush
	FleeAt    float64      `json:"flee_at"`    // hauler burns away until this time, then warps out
	RespondAt float64      `json:"respond_at"` // patrol response due (< 0: never)
	Responded bool         `json:"responded"`
	Outcome   string       `json:"outcome,omitempty"` // "" | destroyed | escaped | saved
}

// Traffic marks a convoy hauler and carries its shipment: the current leg
// (Origin/Dest), the route's final destination (DestSys/DestCel), the
// spawn-time provenance (Item/Qty/Basis), and the live ambush state (nil
// until attacked).
type Traffic struct {
	Origin content.CelID  `json:"origin"`
	Dest   content.CelID  `json:"dest"`
	Item   content.ItemID `json:"item"`
	Qty    int            `json:"qty"`
	Phase  string         `json:"phase"`
	Rolled bool           `json:"rolled"` // the per-leg attack roll was consumed
	Next   float64        `json:"next"`   // earliest next warp (re)attempt (sim time)
	Attack *TrafficAttack `json:"attack,omitempty"`
	// Basis is the shipment's cost basis (€/unit), fixed when the cargo is
	// SOURCED at spawn: the origin station's weighted ask price, or the
	// item's base price for belt and gate (import) origins. The origin book
	// (or belt state) was already debited then, so delivery only posts and
	// a loss never reverses anything.
	Basis float64 `json:"basis,omitempty"`
	// DestSys/DestCel are the route's FINAL destination (system +
	// celestial); Dest is the current leg's target — a next-hop gate while
	// DestSys is still ahead, the destination itself on the last leg. An
	// empty DestSys reads as the spawn system (pre-cross-system saves).
	DestSys content.SystemID `json:"dsys,omitempty"`
	DestCel content.CelID    `json:"dcel,omitempty"`
}

// TrafficJourney is a shipment parked between grids: it hopped a gate (or
// the grid was rebuilt under it) and flies again when System loads next.
// Park order is the canonical serialization order (Save.TrafficJourneys);
// a resumed journey spawns a hauler at At carrying the recorded provenance.
type TrafficJourney struct {
	System  content.SystemID `json:"system"` // system the continuation flies in
	At      core.Vec2        `json:"at"`     // spawn position there (entry gate or mid-leg)
	DestSys content.SystemID `json:"dsys"`   // final destination system
	DestCel content.CelID    `json:"dcel"`   // final destination celestial (in DestSys)
	Item    content.ItemID   `json:"item"`
	Qty     int              `json:"qty"`
	Basis   float64          `json:"basis"` // cost basis carried from spawn-time sourcing
	Hull    content.ShipID   `json:"hull"`
	Rolled  bool             `json:"rolled"` // the parked leg's ambush roll was consumed
	Next    float64          `json:"next"`   // warp retry cooldown (sim time)
}

// Patrol response states (Patrol.State).
const (
	PatrolIdle    = "idle"
	PatrolRespond = "respond"
	PatrolReturn  = "return"
)

// Patrol is the living-universe state of a security patrol ship: idling
// around its anchor (driven by the NPC state machine), warping to an
// incident to break up an ambush, or returning home. Incident is the
// ambushed hauler's entity handle.
type Patrol struct {
	State    string     `json:"state"`
	Incident ecs.Entity `json:"incident,omitempty"`
	Next     float64    `json:"next"` // warp retry cooldown (sim time)
}

// ---------------------------------------------------------------------------
// Commands — the player/agent control surface
// ---------------------------------------------------------------------------

// Command is a player or agent instruction consumed at the start of Step.
type Command interface{ isCommand() }

type (
	// CmdStop cancels movement intent.
	CmdStop struct{}

	// CmdManualVel directly sets sublight velocity direction/throttle.
	CmdManualVel struct {
		Dir       core.Vec2
		SpeedFrac float64
	}

	// CmdApproach flies toward the target and stops at ~0 m.
	CmdApproach struct{ Target ecs.Entity }

	// CmdOrbit circles the target at the given range.
	CmdOrbit struct {
		Target ecs.Entity
		Range  float64
	}

	// CmdWarpTo warps to the target celestial/entity. Range > 0 lands that
	// many metres short of the target along the warp vector, 0 lands on it;
	// the 2500 m landing tolerance still applies around the landing point.
	CmdWarpTo struct {
		Target ecs.Entity
		Range  float64
	}

	// CmdAlignTo aligns and accelerates toward the target without warping.
	CmdAlignTo struct{ Target ecs.Entity }

	// CmdLock starts locking the target.
	CmdLock struct{ Target ecs.Entity }

	// CmdUnlock drops the target lock.
	CmdUnlock struct{ Target ecs.Entity }

	// CmdActivate runs a module cycle (target zero = self).
	CmdActivate struct {
		Slot   SlotRef
		Target ecs.Entity
	}

	// CmdDeactivate stops a module after the current cycle.
	CmdDeactivate struct{ Slot SlotRef }

	// CmdLoot moves loot (Item empty = everything) from wreck to cargo.
	CmdLoot struct {
		Wreck ecs.Entity
		Item  content.ItemID
	}

	// CmdMine activates a mining laser on an asteroid.
	CmdMine struct {
		Asteroid ecs.Entity
		Slot     SlotRef
	}

	// CmdDock docks at a station.
	CmdDock struct{ Station ecs.Entity }

	// CmdUndock leaves the current station.
	CmdUndock struct{}

	// CmdJump activates a stargate.
	CmdJump struct{ Gate ecs.Entity }

	// CmdBuy purchases from the station market while docked.
	CmdBuy struct {
		Item content.ItemID
		Qty  int
	}

	// CmdSell sells from cargo/hangar while docked.
	CmdSell struct {
		Item content.ItemID
		Qty  int
	}

	// CmdRefine refines ore into minerals at a station.
	CmdRefine struct {
		Item content.ItemID
		Qty  int
	}

	// CmdRepair repairs all three layers at a station for a fee.
	CmdRepair struct{}

	// CmdAssemble swaps the active hull for an owned ship in the hangar.
	CmdAssemble struct{ Ship content.ShipID }

	// CmdFitModule fits a module from the hangar into a slot.
	CmdFitModule struct {
		Item content.ModuleID
		Slot SlotRef
	}

	// CmdUnfitModule moves a fitted module to the hangar.
	CmdUnfitModule struct{ Slot SlotRef }

	// CmdSetAmmo sets the preferred charge item for a weapon slot. The gun
	// consumes this exact item first while it is in cargo, then falls back to
	// any charge of its AmmoClass. Item "" restores automatic behaviour
	// (first stack in canonical order). Emits EvModule.
	CmdSetAmmo struct {
		Slot SlotRef
		Item content.ItemID
	}

	// CmdTransfer moves item units between the player's storage holds.
	// From/To are "cargo" (general hold), "ore" (ore hold) or "hangar"
	// (station hangar; requires being docked at a station with the "fitting"
	// service). The UI inventory panels drive it.
	CmdTransfer struct {
		Item content.ItemID
		Qty  int
		From string
		To   string
	}

	// CmdAcceptMission accepts a contract from the offering station's
	// mission board (requires being docked at that station with enough
	// standing). Emitted refusals carry EvMission phase "refused".
	CmdAcceptMission struct{ ID content.ContractID }

	// CmdAbandonMission drops an accepted contract (docked at the offering
	// station). Emitted refusals carry EvMission phase "refused".
	CmdAbandonMission struct{ ID content.ContractID }

	// CmdStartJob starts a manufacturing job at the docked station (which
	// must offer the industry service): Runs sequential runs of Recipe,
	// materials committed up front. Emitted refusals are SevWarning
	// "Industry: …" log lines (see Sim.StartJob).
	CmdStartJob struct {
		Recipe content.RecipeID
		Runs   int
	}

	// CmdCollectJob delivers a finished job's output into the player's
	// hangar (docked at the job's station, industry service).
	CmdCollectJob struct{ ID uint64 }
)

func (CmdStop) isCommand()           {}
func (CmdManualVel) isCommand()      {}
func (CmdApproach) isCommand()       {}
func (CmdOrbit) isCommand()          {}
func (CmdWarpTo) isCommand()         {}
func (CmdAlignTo) isCommand()        {}
func (CmdLock) isCommand()           {}
func (CmdUnlock) isCommand()         {}
func (CmdActivate) isCommand()       {}
func (CmdDeactivate) isCommand()     {}
func (CmdLoot) isCommand()           {}
func (CmdMine) isCommand()           {}
func (CmdDock) isCommand()           {}
func (CmdUndock) isCommand()         {}
func (CmdJump) isCommand()           {}
func (CmdBuy) isCommand()            {}
func (CmdSell) isCommand()           {}
func (CmdRefine) isCommand()         {}
func (CmdRepair) isCommand()         {}
func (CmdAssemble) isCommand()       {}
func (CmdFitModule) isCommand()      {}
func (CmdUnfitModule) isCommand()    {}
func (CmdSetAmmo) isCommand()        {}
func (CmdTransfer) isCommand()       {}
func (CmdAcceptMission) isCommand()  {}
func (CmdAbandonMission) isCommand() {}
func (CmdStartJob) isCommand()       {}
func (CmdCollectJob) isCommand()     {}

// ---------------------------------------------------------------------------
// Events — the observation surface
// ---------------------------------------------------------------------------

// EventKind classifies an event.
type EventKind string

const (
	EvLog            EventKind = "log"
	EvDestroyed      EventKind = "destroyed"
	EvWarp           EventKind = "warp"
	EvLooted         EventKind = "looted"
	EvDocked         EventKind = "docked"
	EvUndocked       EventKind = "undocked"
	EvJumped         EventKind = "jumped"
	EvMarket         EventKind = "market"
	EvModule         EventKind = "module"
	EvTargetLocked   EventKind = "target_locked"
	EvCargoFull      EventKind = "cargo_full"
	EvAnomalyCleared EventKind = "anomaly_cleared"
	EvMission        EventKind = "mission"
	EvTraffic        EventKind = "traffic"
)

// EvTraffic phases: "departed" | "jumped" | "delivered" | "attacked" |
// "destroyed" | "escaped" | "patrol-response" | "saved". Event.System carries
// the system ID and Event.Cel the route endpoint / incident celestial.
const (
	TrafficDeparted = "departed"
	// TrafficJumped is the cross-system gate hop: the hauler vanished
	// through the gate and its shipment parked as a journey (see
	// Save.TrafficJourneys) for the far system to continue.
	TrafficJumped         = "jumped"
	TrafficDelivered      = "delivered"
	TrafficAttacked       = "attacked"
	TrafficDestroyed      = "destroyed"
	TrafficEscaped        = "escaped"
	TrafficPatrolResponse = "patrol-response"
	// TrafficSaved is the "saved" outcome phase: the ambush resolved in the
	// convoy's favour (every raider gone while the hauler lives). Same
	// string as TrafficOutcomeSaved — the event phase and the
	// TrafficAttack.Outcome value are one registry entry.
	TrafficSaved = TrafficOutcomeSaved
)

// EvMission phases: "accepted" | "progress" | "ready" | "completed" |
// "abandoned" | "refused".
const (
	MissionAccepted  = "accepted"
	MissionProgress  = "progress"
	MissionReady     = "ready"
	MissionCompleted = "completed"
	MissionAbandoned = "abandoned"
	MissionRefused   = "refused"
)

// Severity classifies log importance for the UI.
type Severity int

const (
	SevInfo Severity = iota
	SevGood
	SevWarning
	SevDanger
)

// Event is something that happened on the grid or in a service.
type Event struct {
	Kind     EventKind
	Severity Severity
	Time     float64
	Actor    ecs.Entity
	Target   ecs.Entity
	Text     string  // human-readable log line
	Value    float64 // euro, damage, volume... context-dependent
	Item     content.ItemID
	Phase    string           // EvWarp phases: "start" | "land" | "scrammed"; EvMission/EvTraffic phases above
	ID       string           // EvMission: contract ID
	Name     string           // EvMission: contract title
	System   content.SystemID // EvTraffic/EvMarket: the system the move happened in
	Cel      content.CelID    // EvTraffic: route endpoint / incident celestial
}

// ---------------------------------------------------------------------------
// Read model for UI/render
// ---------------------------------------------------------------------------

// Entity kinds reported by Info.Kind.
const (
	KindPlayer   = "player"
	KindShip     = "ship"
	KindAsteroid = "asteroid"
	KindGate     = "gate"
	KindStation  = "station"
	KindWreck    = "wreck"
	KindStar     = "star"
	KindPlanet   = "planet"
	KindMoon     = "moon"
	KindAnomaly  = "anomaly"
)

// KindBelt is the Info.Kind of a belt marker celestial entity.
const KindBelt = "belt"

// SensorRange is the on-grid sensor radius (m): ships, asteroids and wrecks
// are Visible to the player only within this distance. System landmarks
// (star, planet, moon, station, gate, belt and anomaly markers) are always
// visible and warpable.
const SensorRange = 250_000.0

// Info is a flattened view of a grid entity for overview/UI/render.
type Info struct {
	Entity   ecs.Entity
	Name     string
	Kind     string
	Cel      content.CelID // owning celestial (belts/anomalies/gates/stations)
	Pos      core.Vec2
	Vel      core.Vec2
	Distance float64 // from the player (m)
	Hostile  bool    // NPC that is or would be aggressive here
	Health   float64 // structure+armor+shield fraction 0..1 (ships); 1 otherwise
	CanDock  bool
	CanJump  bool
	CanLoot  bool
	CanMine  bool
	CanWarp  bool
	Visible  bool // landmark celestial, or within SensorRange of the player
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// Hangar is the player's station hangar (single global hangar — documented
// simplification of EVE's per-station hangars).
type Hangar struct {
	Ships []StoredShip         `json:"ships"`
	Items []content.CargoStack `json:"items"`
}

// StoredShip is an unassembled hull with its fit and stored cargo.
type StoredShip struct {
	Hull  content.ShipID       `json:"hull"`
	Fit   []content.ModuleID   `json:"fit"`
	Cargo []content.CargoStack `json:"cargo"`
}

// ---------------------------------------------------------------------------
// Market — per-station continuous double auction
// ---------------------------------------------------------------------------

// StationID addresses a station's order book by the station's celestial
// ID. Books are keyed universe-wide by it (content validation rejects a
// station id used by two systems).
type StationID = content.CelID

// OrderSide is one half of an order book.
type OrderSide string

const (
	OrderSell OrderSide = "sell" // ask: offer to sell (rests high)
	OrderBuy  OrderSide = "buy"  // bid: offer to buy (rests low)
)

// Order-owner tags. The player's own orders escrow wallet/inventory at
// placement; every other owner (station founding orders, convoy supply,
// future agents) is the system side of the trade.
const (
	OwnerPlayer = "player"
	OwnerSystem = "system"
)

// MarketStock is one resting order on a station's book. Qty keeps its old
// "remaining stock" reading: the unfilled remainder of the order.
type MarketStock struct {
	Item    content.ItemID `json:"item"`
	Price   float64        `json:"price"`
	Qty     int            `json:"qty"`
	Side    OrderSide      `json:"side"`
	Owner   string         `json:"owner,omitempty"` // opaque ledger tag ("" reads as the system side)
	Station StationID      `json:"station"`         // book the order rests on
	ID      uint64         `json:"id"`              // arrival id: the price-time priority tiebreak
}

// SavedLastTrade is one station/item last trade price — the price ladder's
// top rung, persisted per book alongside the orders.
type SavedLastTrade struct {
	System  content.SystemID `json:"system"`
	Station StationID        `json:"station"`
	Item    content.ItemID   `json:"item"`
	Price   float64          `json:"price"`
}

// MarketBook is one station's order book. Orders stay in canonical order —
// item ascending, asks before bids, asks by price ascending, bids by price
// descending, arrival id breaking ties — so every read and every save is a
// pure function of state. Lasts is the last trade price per item (the top
// rung of the price ladder); it is point-looked-up only, never iterated.
type MarketBook struct {
	System content.SystemID
	Orders []MarketStock
	Lasts  map[content.ItemID]float64
}

// sortOrders puts the book into canonical order (see MarketBook).
func (b *MarketBook) sortOrders() {
	sideRank := func(s OrderSide) int {
		if s == OrderSell {
			return 0
		}
		return 1
	}
	sort.Slice(b.Orders, func(i, j int) bool { return orderLess(b.Orders[i], b.Orders[j], sideRank) })
}

// orderLess is the canonical book order: (item, side, price, id). Asks
// rank first and sort ascending, bids rank second and sort descending, so
// the front of each side is that side's best price.
func orderLess(a, c MarketStock, sideRank func(OrderSide) int) bool {
	if a.Item != c.Item {
		return a.Item < c.Item
	}
	ra, rc := sideRank(a.Side), sideRank(c.Side)
	if ra != rc {
		return ra < rc
	}
	if a.Price != c.Price {
		if ra == 0 { // ask: best (lowest) first
			return a.Price < c.Price
		}
		return a.Price > c.Price // bid: best (highest) first
	}
	return a.ID < c.ID
}

// bestAsk returns the cheapest sell order for item (price-time priority).
func (b *MarketBook) bestAsk(item content.ItemID) (MarketStock, bool) {
	return b.best(item, OrderSell)
}

// bestBid returns the highest buy order for item (price-time priority).
func (b *MarketBook) bestBid(item content.ItemID) (MarketStock, bool) {
	return b.best(item, OrderBuy)
}

func (b *MarketBook) best(item content.ItemID, side OrderSide) (MarketStock, bool) {
	for _, o := range b.Orders { // canonical: the first match is the best
		if o.Item == item && o.Side == side {
			return o, true
		}
	}
	return MarketStock{}, false
}

// match is one executed trade fragment against a resting order: the
// resting order's price always wins, qty is what the resting order had.
type match struct {
	order MarketStock
	qty   int
}

// matchCrossing walks the opposite side for item in price-time priority
// and returns the resting orders an incoming order would cross, with the
// quantity taken from each. limit is the worst price the incoming order
// accepts; <= 0 means a market order (no price limit).
func (b *MarketBook) matchCrossing(side OrderSide, item content.ItemID, qty int, limit float64) []match {
	var out []match
	for _, o := range b.Orders {
		if o.Item != item || o.Side == side || o.Qty <= 0 {
			continue
		}
		if limit > 0 {
			if side == OrderBuy && o.Price > limit {
				continue
			}
			if side == OrderSell && o.Price < limit {
				continue
			}
		}
		n := min(qty, o.Qty)
		out = append(out, match{order: o, qty: n})
		qty -= n
		if qty <= 0 {
			break
		}
	}
	return out
}

// applyMatches decrements the matched resting orders by their fills and
// drops the exhausted ones, keeping the book canonical.
func (b *MarketBook) applyMatches(ms []match) {
	if len(ms) == 0 {
		return
	}
	filled := make(map[uint64]int, len(ms))
	for _, m := range ms {
		filled[m.order.ID] += m.qty
	}
	kept := b.Orders[:0]
	for _, o := range b.Orders {
		if n := filled[o.ID]; n > 0 {
			o.Qty -= n
			filled[o.ID] = 0
		}
		if o.Qty > 0 {
			kept = append(kept, o)
		}
	}
	b.Orders = kept
	b.sortOrders()
}

// lastPrice returns the item's last trade price (the ladder's top rung).
func (b *MarketBook) lastPrice(item content.ItemID) (float64, bool) {
	p, ok := b.Lasts[item]
	return p, ok
}

// OrderResult reports what one PlaceOrder did: how much of the order
// executed on arrival, at what value, what fees were charged and what
// escrow came back, and the resting remainder's arrival id.
type OrderResult struct {
	ID        uint64  // arrival id of the resting remainder (0 when fully filled)
	Filled    int     // units executed on arrival
	Cost      float64 // executed value (before fees and tax)
	Fee       float64 // broker fee charged at placement (player buy side)
	Refund    float64 // escrow returned: unfilled remainder + price improvement
	Remaining int     // units resting after placement
}

// RespawnTimer is a pending NPC respawn.
type RespawnTimer struct {
	System content.SystemID `json:"system"`
	Cel    content.CelID    `json:"cel"`
	Npc    content.NpcID    `json:"npc"`
	At     float64          `json:"at"`
}

// TrafficLeave schedules one ambushing NPC's break-off: at the saved sim
// time the raider disengages and leaves, whether or not the hauler whose
// ambush it was still exists.
type TrafficLeave struct {
	Entity ecs.Entity `json:"entity"`
	At     float64    `json:"at"`
}

// SavedEntity is one entity's full component snapshot (nil = absent).
type SavedEntity struct {
	ID        uint64      `json:"id"` // raw ecs.Entity
	Name      *string     `json:"name,omitempty"`
	Transform *Transform  `json:"transform,omitempty"`
	Motion    *Motion     `json:"motion,omitempty"`
	Ship      *ShipState  `json:"ship,omitempty"`
	Cap       *Capacitor  `json:"cap,omitempty"`
	Cargo     *Cargo      `json:"cargo,omitempty"`
	Fitting   *Fitting    `json:"fitting,omitempty"`
	Targeting *Targeting  `json:"targeting,omitempty"`
	Pilot     *Pilot      `json:"pilot,omitempty"`
	NPC       *SavedNpcAI `json:"npc,omitempty"`
	Asteroid  *Asteroid   `json:"asteroid,omitempty"`
	Celestial *Celestial  `json:"celestial,omitempty"`
	Gate      *Stargate   `json:"gate,omitempty"`
	Station   *Station    `json:"station,omitempty"`
	Docked    *Docked     `json:"docked,omitempty"`
	Warp      *WarpDrive  `json:"warp,omitempty"`
	Wreck     *Wreck      `json:"wreck,omitempty"`
	Intent    *Intent     `json:"intent,omitempty"`  // movement intent
	Traffic   *Traffic    `json:"traffic,omitempty"` // convoy hauler leg/ambush state
	Patrol    *Patrol     `json:"patrol,omitempty"`  // security patrol response state
}

// SavedNpcAI is NpcAI with serializable RNG state.
type SavedNpcAI struct {
	Npc       content.NpcID `json:"npc"`
	State     NpcAIState    `json:"state"`
	Target    uint64        `json:"target"`
	Home      core.Vec2     `json:"home"`
	RandState uint64        `json:"rand_state"`
	// LastHitBy is NpcAI.LastHitBy as a handle (omitempty: a never-hit NPC
	// serializes exactly as before the field existed).
	LastHitBy uint64 `json:"last_hit_by,omitempty"`
}

// SavedModTarget is an active-module target binding.
type SavedModTarget struct {
	Owner  uint64 `json:"owner"`
	Class  string `json:"class"`
	Index  int    `json:"index"`
	Target uint64 `json:"target"`
}

// SavedSlot mirrors one ecs free-list entry (slot index + generation) so
// restored worlds hand out identical future Entity handles.
type SavedSlot struct {
	Index      uint32 `json:"index"`
	Generation uint32 `json:"generation"`
}

// MissionState is one accepted contract's live progress: the contract ID and
// how much of its objective has been counted (kills, m³ delivered/mined —
// deliver counts stay 0 until the claim check at the destination). Phase is
// derived: Progress < count is in-progress, otherwise ready to claim.
// Persisted verbatim in Save.Missions.
type MissionState struct {
	ID       content.ContractID `json:"id"`
	Progress float64            `json:"progress"`
}

// saveSchemaV2 is the save-schema version written since the per-station
// market tranche (v0.2.0 era): order books, order ids, last-trade prices.
// Restore rejects any other value — the clean-break policy for pre-v2
// saves, which carried per-system stock lists with drifting prices.
const saveSchemaV2 = 2

// Save is a full serializable simulation snapshot.
type Save struct {
	World    content.WorldID  `json:"world"`
	System   content.SystemID `json:"system"`
	Ticks    int64            `json:"ticks"`
	Seed     int64            `json:"seed"`
	RNGState uint64           `json:"rng_state"`
	// TrafficRNGState is the state of the living-universe RNG stream
	// (traffic spawns, ambush rolls, patrol scatter), forked from the master
	// stream so convoy activity never shifts the player's combat rolls.
	TrafficRNGState uint64  `json:"traffic_rng_state"`
	Wallet          float64 `json:"wallet"`
	// Standing is the player's career standing (contract completion credit).
	Standing float64 `json:"standing"`
	// Missions are the contracts currently accepted (with progress).
	Missions []MissionState `json:"missions,omitempty"`
	Hangar   Hangar         `json:"hangar"`
	// Schema is the save-schema version (saveSchemaV2): Restore rejects any
	// other value, so pre-v2 saves — which carried per-system stock lists
	// and drifting prices instead of per-station order books — fail loudly
	// instead of restoring into a different market.
	Schema int `json:"schema"`
	// Market carries every resting order, grouped by the system its station
	// lives in; each order names its own station, side, owner and arrival
	// id. Canonical order: station id ascending, then the book's canonical
	// order — the per-station books of the v2 market schema.
	Market map[content.SystemID][]MarketStock `json:"market"`
	// MarketLasts is the last trade price per (station, item), sorted by
	// station then item: the rung of the price ladder the orders cannot
	// reconstruct (an all-filled book still remembers its last trade).
	MarketLasts []SavedLastTrade `json:"market_lasts,omitempty"`
	// MarketSeq is the next order arrival id (price-time priority clock).
	MarketSeq uint64         `json:"market_seq"`
	Respawn   []RespawnTimer `json:"respawn"`
	// TrafficLeaves are scheduled raider break-offs (living-universe
	// ambushes resolved before their raiders left the field).
	TrafficLeaves []TrafficLeave `json:"traffic_leaves,omitempty"`
	Entities      []SavedEntity  `json:"entities"`
	// ModTargets are the active-module target bindings, kept sorted by
	// owner/class/index so map iteration order never leaks into the
	// canonical serialization (Hash).
	ModTargets []SavedModTarget `json:"mod_targets,omitempty"`
	// FreeSlots mirrors the ECS free list in storage order so restored
	// worlds hand out identical future Entity handles.
	FreeSlots []SavedSlot `json:"free_slots,omitempty"`
	// LastStation is the entity handle of the last docked station (the
	// respawn anchor); 0 when that station is not on the current grid.
	LastStation uint64 `json:"last_station,omitempty"`
	// LastStationCel is the respawn anchor as a celestial ID, persisted
	// unconditionally: a player who saves while anchored in another system
	// has no on-grid handle (LastStation 0) but must still respawn at that
	// station on return. Restore prefers this field and falls back to the
	// handle for saves written before it existed.
	LastStationCel content.CelID `json:"last_station_cel,omitempty"`

	// ---- world metabolism: persistent belts + faction ledgers ----

	// Belts is every belt's persistent per-rock remaining volume, in
	// canonical order: system id ascending, then celestial id ascending
	// (rocks in BeltDef order). Seeded from the BeltDef on first
	// generation, decremented by mining and rat requisitions, regrown by
	// the 600-tick window — belt depletion survives reloads.
	Belts []BeltState `json:"belts,omitempty"`
	// FactionLedger is the per-faction requisition balance NPC replacements
	// draw on when they buy hulls, modules, charges or ore through a
	// station book (a real sink: only respawn requisitions debit it).
	// Map keys marshal sorted, so the serialization stays canonical.
	FactionLedger map[string]float64 `json:"faction_ledger,omitempty"`

	// --- industry (manufacturing jobs + station craft growth) ---
	// Jobs are the station manufacturing jobs (running or awaiting
	// collection), kept in ascending job-id order — the canonical
	// iteration for restore, save and Hash.
	Jobs []Job `json:"jobs,omitempty"`
	// JobSeq is the next job id (the market's MarketSeq pattern).
	JobSeq uint64 `json:"job_seq"`
	// Craft is each station's per-family craft counter (completed runs +
	// idle ticks), sorted by (station, family) — the CraftLevel input.
	Craft []SavedCraft `json:"craft,omitempty"`

	// ---- ledger agents (system 18) ----

	// Agents is the roster in ascending id order: capital, inventory,
	// EWMA beliefs, ZIP margins and haul wishes per agent (inventory,
	// beliefs and margins serialized as sorted (item, value) pairs).
	// Resting-order ledgers are derived from Market on restore — a Save
	// lands on Step boundaries, where every fill has settled.
	Agents []SavedAgent `json:"agents,omitempty"`
	// Upkeep is the per-station maintenance ledger (money sink) behind
	// the periodic repair/rearm basket. Map keys marshal sorted, so the
	// serialization stays canonical.
	Upkeep map[StationID]float64 `json:"upkeep,omitempty"`

	// ---- logistics: shipments parked between grids ----
	// TrafficJourneys are convoys waiting to continue in another system
	// (a gate hop, or the grid rebuilt under them mid-leg), in park order —
	// the canonical serialization (Hash) of the journey ledger.
	TrafficJourneys []TrafficJourney `json:"traffic_journeys,omitempty"`
}
