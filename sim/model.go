// Package sim is Entieve's deterministic game simulation: components,
// commands, events and the systems that advance them.
//
// sim never imports Ebitengine. It is fully drivable headlessly: AI agents
// and tests issue Command values through Queue and read Event values back —
// this is the engine's controllability seam.
package sim

import (
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
}

// Asteroid is a minable rock.
type Asteroid struct {
	Ore       content.ItemID
	Remaining float64 // m³ left
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
}

// Wreck is the remains of a destroyed ship, holding rolled loot.
type Wreck struct {
	Loot      []content.CargoStack
	ExpiresAt float64
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

	// CmdWarpTo warps to the target celestial/entity.
	CmdWarpTo struct{ Target ecs.Entity }

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
)

func (CmdStop) isCommand()        {}
func (CmdManualVel) isCommand()   {}
func (CmdApproach) isCommand()    {}
func (CmdOrbit) isCommand()       {}
func (CmdWarpTo) isCommand()      {}
func (CmdAlignTo) isCommand()     {}
func (CmdLock) isCommand()        {}
func (CmdUnlock) isCommand()      {}
func (CmdActivate) isCommand()    {}
func (CmdDeactivate) isCommand()  {}
func (CmdLoot) isCommand()        {}
func (CmdMine) isCommand()        {}
func (CmdDock) isCommand()        {}
func (CmdUndock) isCommand()      {}
func (CmdJump) isCommand()        {}
func (CmdBuy) isCommand()         {}
func (CmdSell) isCommand()        {}
func (CmdRefine) isCommand()      {}
func (CmdRepair) isCommand()      {}
func (CmdAssemble) isCommand()    {}
func (CmdFitModule) isCommand()   {}
func (CmdUnfitModule) isCommand() {}
func (CmdTransfer) isCommand()    {}

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
	Value    float64 // credits, damage, volume... context-dependent
	Item     content.ItemID
	Phase    string // EvWarp phases: "start" | "land" | "scrammed"
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

// MarketStock is remaining stock of one market order.
type MarketStock struct {
	Item  content.ItemID `json:"item"`
	Price float64        `json:"price"`
	Qty   int            `json:"qty"`
}

// RespawnTimer is a pending NPC respawn.
type RespawnTimer struct {
	System content.SystemID `json:"system"`
	Cel    content.CelID    `json:"cel"`
	Npc    content.NpcID    `json:"npc"`
	At     float64          `json:"at"`
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
	Intent    *Intent     `json:"intent,omitempty"` // movement intent
}

// SavedNpcAI is NpcAI with serializable RNG state.
type SavedNpcAI struct {
	Npc       content.NpcID `json:"npc"`
	State     NpcAIState    `json:"state"`
	Target    uint64        `json:"target"`
	Home      core.Vec2     `json:"home"`
	RandState uint64        `json:"rand_state"`
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

// Save is a full serializable simulation snapshot.
type Save struct {
	World    content.WorldID                    `json:"world"`
	System   content.SystemID                   `json:"system"`
	Ticks    int64                              `json:"ticks"`
	Seed     int64                              `json:"seed"`
	RNGState uint64                             `json:"rng_state"`
	Wallet   float64                            `json:"wallet"`
	Hangar   Hangar                             `json:"hangar"`
	Market   map[content.SystemID][]MarketStock `json:"market"`
	Respawn  []RespawnTimer                     `json:"respawn"`
	Entities []SavedEntity                      `json:"entities"`
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
}
