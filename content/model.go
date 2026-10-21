// Package content defines Entieve's data schema and loads game data from
// JSON files under content/data.
//
// All game content (items, ships, modules, NPCs, star systems, worlds) is
// described by the structs in this file. Content authors — including AI
// agents — add content by writing JSON files; see docs/CONTENT_AUTHORING.md.
package content

import (
	"strings"

	"github.com/entieve/entieve/core"
)

// Typed IDs. All IDs are kebab-case strings, globally unique per kind.
type (
	ItemID   string
	ShipID   string
	ModuleID string
	NpcID    string
	CelID    string
	SystemID string
	WorldID  string
)

// DamageType is one of EVE's four canonical damage types.
type DamageType string

const (
	DmgEM        DamageType = "em"
	DmgThermal   DamageType = "thermal"
	DmgKinetic   DamageType = "kinetic"
	DmgExplosive DamageType = "explosive"
)

// AllDamageTypes lists the damage types in canonical order.
var AllDamageTypes = []DamageType{DmgEM, DmgThermal, DmgKinetic, DmgExplosive}

// Damage is raw damage per type, in hit points.
type Damage struct {
	EM        float64 `json:"em"`
	Thermal   float64 `json:"thermal"`
	Kinetic   float64 `json:"kinetic"`
	Explosive float64 `json:"explosive"`
}

// Of returns the damage of the given type.
func (d Damage) Of(t DamageType) float64 {
	switch t {
	case DmgEM:
		return d.EM
	case DmgThermal:
		return d.Thermal
	case DmgKinetic:
		return d.Kinetic
	case DmgExplosive:
		return d.Explosive
	}
	return 0
}

// Total returns the sum over all damage types.
func (d Damage) Total() float64 {
	return d.EM + d.Thermal + d.Kinetic + d.Explosive
}

// Scale returns d multiplied by k, per type.
func (d Damage) Scale(k float64) Damage {
	return Damage{d.EM * k, d.Thermal * k, d.Kinetic * k, d.Explosive * k}
}

// Add returns the per-type sum of d and o.
func (d Damage) Add(o Damage) Damage {
	return Damage{d.EM + o.EM, d.Thermal + o.Thermal, d.Kinetic + o.Kinetic, d.Explosive + o.Explosive}
}

// Resists are per-type damage reduction fractions, each in [0,1).
type Resists struct {
	EM        float64 `json:"em"`
	Thermal   float64 `json:"thermal"`
	Kinetic   float64 `json:"kinetic"`
	Explosive float64 `json:"explosive"`
}

// Of returns the resist fraction for the given type.
func (r Resists) Of(t DamageType) float64 {
	switch t {
	case DmgEM:
		return r.EM
	case DmgThermal:
		return r.Thermal
	case DmgKinetic:
		return r.Kinetic
	case DmgExplosive:
		return r.Explosive
	}
	return 0
}

// Multiplier returns the fraction of damage that penetrates the resist.
func (r Resists) Multiplier(t DamageType) float64 {
	return 1 - r.Of(t)
}

// ApplyTo reduces d by resists and returns the penetrating damage.
func (d Damage) ApplyTo(r Resists) Damage {
	return Damage{
		EM:        d.EM * r.Multiplier(DmgEM),
		Thermal:   d.Thermal * r.Multiplier(DmgThermal),
		Kinetic:   d.Kinetic * r.Multiplier(DmgKinetic),
		Explosive: d.Explosive * r.Multiplier(DmgExplosive),
	}
}

// AddResists sums resist fractions per type (clamped to <1 at use sites).
func (r Resists) Add(o Resists) Resists {
	return Resists{r.EM + o.EM, r.Thermal + o.Thermal, r.Kinetic + o.Kinetic, r.Explosive + o.Explosive}
}

// CargoStack is a stack of identical items.
type CargoStack struct {
	Item ItemID `json:"item"`
	Qty  int    `json:"qty"`
}

// Item categories.
const (
	CatMineral = "mineral"
	CatOre     = "ore"
	CatAmmo    = "ammo"
	CatCharge  = "charge"
	CatLoot    = "loot"
)

// ItemDef defines a non-ship, non-module item (mineral, ore, ammo, charge, loot).
type ItemDef struct {
	ID          ItemID             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Maker       string             `json:"maker,omitempty"` // manufacturer: a corp-* slug (Corp.ID); empty for ores/minerals/loot
	Category    string             `json:"category"`
	Volume      float64            `json:"volume"`                 // m³ per unit
	BasePrice   float64            `json:"base_price"`             // credits per unit
	RefineYield map[ItemID]float64 `json:"refine_yield,omitempty"` // ore: fraction of one unit refining into minerals, sums <= 1
	Damage      *Damage            `json:"damage,omitempty"`       // ammo: damage per shot
	AmmoClass   string             `json:"ammo_class,omitempty"`   // ammo: the calibre ("small"/"medium"/"large"); "" for non-ammo
}

// Ship slots.
type Slots struct {
	High int `json:"high"`
	Mid  int `json:"mid"`
	Low  int `json:"low"`
}

// ShipDef defines a flyable or NPC hull.
type ShipDef struct {
	ID             ShipID  `json:"id"`
	Name           string  `json:"name"`
	Class          string  `json:"class"` // "frigate"|"destroyer"|"barge"|"industrial"|"rat"
	Description    string  `json:"description,omitempty"`
	Origin         string  `json:"origin,omitempty"` // naming-layer heritage: "moravian"|"polish"|"nordic"|"latinate"|"old-works"
	Maker          string  `json:"maker,omitempty"`  // manufacturer: a corp-* slug (Corp.ID); empty on the unattributed rat hulls
	Slots          Slots   `json:"slots"`
	Shield         float64 `json:"shield"`
	Armor          float64 `json:"armor"`
	Structure      float64 `json:"structure"`
	Resists        Resists `json:"resists"`
	Capacitor      float64 `json:"capacitor"`        // GJ
	Recharge       float64 `json:"recharge"`         // capacitor time constant tau, seconds
	MaxSpeed       float64 `json:"max_speed"`        // m/s
	Agility        float64 `json:"agility"`          // align-time factor; align time = agility * 10 s
	Mass           float64 `json:"mass"`             // kg
	SigRadius      float64 `json:"sig_radius"`       // m
	Cargo          float64 `json:"cargo"`            // m³
	OreHold        float64 `json:"ore_hold"`         // m³; 0 means ore uses cargo
	WarpSpeed      float64 `json:"warp_speed"`       // AU/s
	MaxTargetRange float64 `json:"max_target_range"` // m
	MaxTargets     int     `json:"max_targets"`
	ScanResolution float64 `json:"scan_resolution"` // mm/s-ish locking speed factor
}

// Module slot classes.
const (
	SlotHigh = "high"
	SlotMid  = "mid"
	SlotLow  = "low"
)

// Module kinds.
const (
	KindWeapon         = "weapon"
	KindShieldBoost    = "shield_boost"
	KindShieldExtender = "shield_extender"
	KindArmorRep       = "armor_rep"
	KindArmorPlate     = "armor_plate"
	KindResistCoat     = "resist_coat"
	KindCapBooster     = "cap_booster"
	KindCapRecharger   = "cap_recharger"
	KindAfterburner    = "afterburner"
	KindMiningLaser    = "mining_laser"
	KindStasisWeb      = "stasis_web"
	KindWarpScram      = "warp_scram"
)

// WeaponAttrs are weapon-module parameters.
type WeaponAttrs struct {
	Damage     Damage  `json:"damage"`                      // base volley before ammo; ammo Damage (if any) replaces this per shot
	Optimal    float64 `json:"optimal"`                     // m
	Falloff    float64 `json:"falloff"`                     // m
	Tracking   float64 `json:"tracking"`                    // rad/s
	AmmoClass  string  `json:"ammo_class"`                  // "" = no ammo needed
	ShotTime   float64 `json:"shot_time"`                   // seconds between shots
	DamageMult float64 `json:"damage_multiplier,omitempty"` // ammo shell damage is scaled by this (0 reads as 1)
}

// Mult returns the weapon's damage multiplier: the gun's share of a shot's
// damage, applied to the loaded shell's damage profile.
func (w WeaponAttrs) Mult() float64 {
	if w.DamageMult <= 0 {
		return 1
	}
	return w.DamageMult
}

// MiningAttrs are mining-module parameters.
type MiningAttrs struct {
	VolumePerCycle float64 `json:"volume_per_cycle"` // m³ of asteroid volume per cycle
	CycleTime      float64 `json:"cycle_time"`       // seconds
}

// BoostAttrs are active-module effect parameters (fields used per Kind).
type BoostAttrs struct {
	ShieldBoostPerCycle float64 `json:"shield_boost_per_cycle,omitempty"`
	ArmorRepPerCycle    float64 `json:"armor_rep_per_cycle,omitempty"`
	CapRestore          float64 `json:"cap_restore,omitempty"`      // GJ per activation (cap booster)
	SpeedMultiplier     float64 `json:"speed_multiplier,omitempty"` // afterburner: > 1
	WebStrength         float64 `json:"web_strength,omitempty"`     // 0..1 target speed reduction
	ScramDuration       float64 `json:"scram_duration,omitempty"`   // seconds of warp disruption
}

// PassiveAttrs are passive-module stat additions.
type PassiveAttrs struct {
	ShieldHP    float64 `json:"shield_hp,omitempty"`
	ArmorHP     float64 `json:"armor_hp,omitempty"`
	Capacitor   float64 `json:"capacitor,omitempty"`
	Recharge    float64 `json:"recharge,omitempty"` // seconds added to tau (negative = faster)
	ResistBonus Resists `json:"resist_bonus,omitempty"`
	MaxSpeed    float64 `json:"max_speed,omitempty"` // absolute override addition, m/s
}

// ModuleDef defines a fit-able module.
type ModuleDef struct {
	ID             ModuleID      `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description,omitempty"`
	Maker          string        `json:"maker,omitempty"` // manufacturer: a corp-* slug (Corp.ID)
	Slot           string        `json:"slot"`            // "high" | "mid" | "low"
	Kind           string        `json:"kind"`
	ActivationCost float64       `json:"activation_cost"` // GJ per activation/cycle; 0 for passive
	ActivationTime float64       `json:"activation_time"` // seconds per cycle; 0 for passive
	Passive        *PassiveAttrs `json:"passive,omitempty"`
	Weapon         *WeaponAttrs  `json:"weapon,omitempty"`
	Mining         *MiningAttrs  `json:"mining,omitempty"`
	Boost          *BoostAttrs   `json:"boost,omitempty"`
	Price          float64       `json:"price"` // credits, for market seeding
}

// LootDrop is one entry of an NPC loot table.
type LootDrop struct {
	Item   ItemID  `json:"item"`
	Chance float64 `json:"chance"` // 0..1 per drop roll
	MinQty int     `json:"min_qty"`
	MaxQty int     `json:"max_qty"`
}

// SpawnDef spawns NPCs near a celestial.
type SpawnDef struct {
	NpcID      NpcID   `json:"npc_id"`
	Count      int     `json:"count"`
	RespawnSec float64 `json:"respawn_sec"` // 0 = no respawn
}

// AsteroidDef is one rock in a belt.
type AsteroidDef struct {
	Ore    ItemID  `json:"ore"`
	Count  int     `json:"count"`
	Volume float64 `json:"volume"` // m³ per asteroid
}

// BeltDef groups asteroids.
type BeltDef struct {
	Asteroids []AsteroidDef `json:"asteroids"`
}

// GateDef is a stargate connection (destination verified both ways).
type GateDef struct {
	ToSystem SystemID `json:"to_system"`
	ToCel    CelID    `json:"to_cel"`
}

// StationDef lists station services and the station's founding market
// orders (the economy's initial endowment: the book seeds every station
// starts with; ongoing flow is never conjured).
type StationDef struct {
	Services []string         `json:"services"` // content.Services ids (repair | market | refine | fitting | industry)
	Market   []MarketOrderDef `json:"market,omitempty"`
}

// Celestial kinds.
const (
	CelStar    = "star"
	CelPlanet  = "planet"
	CelMoon    = "moon"
	CelStation = "station"
	CelBelt    = "belt"
	CelGate    = "gate"
	CelAnomaly = "anomaly"
)

// AnomalyDef is a combat site with its own respawn-able spawn group.
type AnomalyDef struct {
	Spawns     []SpawnDef `json:"spawns"`
	RespawnSec float64    `json:"respawn_sec"`
}

// CelestialDef is a place in a star system.
type CelestialDef struct {
	ID       CelID       `json:"id"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Position core.Vec2   `json:"position"`         // system grid space, meters
	Radius   float64     `json:"radius"`           // m
	Colors   []string    `json:"colors,omitempty"` // hex palette for rendering
	Gate     *GateDef    `json:"gate,omitempty"`
	Station  *StationDef `json:"station,omitempty"`
	Belt     *BeltDef    `json:"belt,omitempty"`
	Anomaly  *AnomalyDef `json:"anomaly,omitempty"`
	Spawns   []SpawnDef  `json:"spawns,omitempty"` // ambient spawns at this celestial
}

// MarketOrderDef is one founding order on a station's book (infinite
// restock is NOT used; Qty is finite and depletes with trades). Side
// selects the book half: "sell" (an ask — the default when omitted) or
// "buy" (a bid).
type MarketOrderDef struct {
	Item  ItemID  `json:"item"`
	Qty   int     `json:"qty"`
	Price float64 `json:"price"`
	Side  string  `json:"side,omitempty"` // "" | "sell" | "buy"
}

// SystemDef is a complete star system.
type SystemDef struct {
	ID            SystemID          `json:"id"`
	Name          string            `json:"name"`
	Security      string            `json:"security"` // "high" | "low" | "null" (flavor + NPC aggression)
	Constellation string            `json:"constellation"`
	Region        string            `json:"region"`
	Position      core.Vec2         `json:"position"`          // starmap plane (ly)
	Palette       map[string]string `json:"palette,omitempty"` // rendering colors, e.g. {"nebula1":"#123456","star":"#ffdd88"}
	Celestials    []CelestialDef    `json:"celestials"`
	// Market is the pre-v2 system-level market block. Founding orders live
	// on StationDef.Market now: Validate rejects any system-level block.
	// The field is kept only so legacy schema readers still compile.
	Market []MarketOrderDef `json:"market,omitempty"`
}

// UniverseDef ties systems together.
type UniverseDef struct {
	Region        string      `json:"region"`
	Constellation string      `json:"constellation"`
	StartSystem   SystemID    `json:"start_system"`
	Systems       []SystemDef `json:"systems"` // full inline systems also allowed; file-based systems merge in
}

// ContractID is the ID of a mission-board contract. IDs use the "contract-"
// prefix (kebab-case, globally unique).
type ContractID string

// Contract kinds.
const (
	ContractKill    = "kill"
	ContractDeliver = "deliver"
	ContractMine    = "mine"
)

// ContractObjective is the per-kind objective block of a contract:
// kill    -> Target (NPC id) + Count rats;
// deliver -> Item + Count m³ delivered to Destination (a station);
// mine    -> Item (ore) + Count m³ mined into the hold.
type ContractObjective struct {
	Target      NpcID  `json:"target,omitempty"`
	Item        ItemID `json:"item,omitempty"`
	Destination CelID  `json:"destination,omitempty"`
	Count       int    `json:"count"`
}

// ContractReward is the payout block: credits on completion, an optional
// bonus item (granted to the station hangar), and the career-standing gain.
type ContractReward struct {
	Credits      float64 `json:"credits"`
	Item         ItemID  `json:"item,omitempty"`
	Qty          int     `json:"qty,omitempty"`
	StandingGain float64 `json:"standing_gain"`
}

// ContractDef is one mission-board offering, pinned to the station whose
// board lists it (Station). Kill/mine contracts claim at Station; deliver
// contracts claim at Objective.Destination.
type ContractDef struct {
	ID          ContractID        `json:"id"`
	Title       string            `json:"title"`
	Kind        string            `json:"kind"` // ContractKill | ContractDeliver | ContractMine
	Level       int               `json:"level"`
	MinStanding float64           `json:"min_standing"`
	Station     CelID             `json:"station"`
	Objective   ContractObjective `json:"objective"`
	Reward      ContractReward    `json:"reward"`
	Description string            `json:"description"`
}

// RouteCargo is the shipment carried by one traffic route.
type RouteCargo struct {
	Item ItemID `json:"item"`
	Qty  int    `json:"qty"`
}

// RouteDef is one one-way convoy leg: a hauler spawns at From, warps the
// leg (first to the next-hop gate when the route crosses systems), delivers
// Cargo at To when To is a station, and despawns when To is a same-system
// gate (the hauler leaves the system) or belt. ToSystem names the
// destination system; empty means this def's own system — the original
// in-system routes.
type RouteDef struct {
	From          CelID      `json:"from"`
	To            CelID      `json:"to"`
	ToSystem      SystemID   `json:"to_system,omitempty"`
	Hull          ShipID     `json:"hull"`
	Cargo         RouteCargo `json:"cargo"`
	IntervalTicks int64      `json:"interval_ticks"` // authored cadence; security scales it (docs/CONTENT_AUTHORING.md)
}

// PatrolDef configures a system's security patrols: the wing composition
// and the authored spawn cadence (security scales it).
type PatrolDef struct {
	Hulls         []ShipID `json:"hulls"`
	IntervalTicks int64    `json:"interval_ticks"`
}

// TrafficDef is one system's living-universe traffic configuration: convoy
// routes, the patrol wing, and (in low/null security) the local rat class
// that ambushes convoys. Loaded from content/data/traffic/, one def per
// system; the system ID is the def's identity.
type TrafficDef struct {
	System SystemID   `json:"system"`
	Routes []RouteDef `json:"routes"`
	Patrol *PatrolDef `json:"patrol,omitempty"`
	Raider NpcID      `json:"raider,omitempty"` // required when security is low|null
}

// Checkpoint documents one feature exercised by a showcase scenario.
type Checkpoint struct {
	Name    string `json:"name"`
	Feature string `json:"feature"`
	Detail  string `json:"detail"`
}

// WorldDef is a playable scenario.
type WorldDef struct {
	ID          WorldID      `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Start       StartDef     `json:"start"`
	Scenario    []Checkpoint `json:"scenario,omitempty"`
}

// StartDef positions the player at world start.
type StartDef struct {
	System   SystemID     `json:"system"`
	Position core.Vec2    `json:"position"`
	Ship     ShipID       `json:"ship"`
	Fit      []ModuleID   `json:"fit"` // high slots first, then mid, then low (validated)
	Cargo    []CargoStack `json:"cargo"`
	Wallet   float64      `json:"wallet"`
}

// Content is the fully loaded, cross-referenced game data.
type Content struct {
	Items     map[ItemID]*ItemDef
	Ships     map[ShipID]*ShipDef
	Modules   map[ModuleID]*ModuleDef
	NPCs      map[NpcID]*NpcDef
	Systems   map[SystemID]*SystemDef
	Contracts map[ContractID]*ContractDef
	Traffic   map[SystemID]*TrafficDef
	Universe  *UniverseDef
	Worlds    map[WorldID]*WorldDef
	Recipes   map[RecipeID]*RecipeDef
}

// Item returns the item def for id.
func (c *Content) Item(id ItemID) (*ItemDef, bool) { d, ok := c.Items[id]; return d, ok }

// Ship returns the ship def for id.
func (c *Content) Ship(id ShipID) (*ShipDef, bool) { d, ok := c.Ships[id]; return d, ok }

// Module returns the module def for id.
func (c *Content) Module(id ModuleID) (*ModuleDef, bool) { d, ok := c.Modules[id]; return d, ok }

// Npc returns the NPC def for id.
func (c *Content) Npc(id NpcID) (*NpcDef, bool) { d, ok := c.NPCs[id]; return d, ok }

// System returns the system def for id.
func (c *Content) System(id SystemID) (*SystemDef, bool) { d, ok := c.Systems[id]; return d, ok }

// World returns the world def for id.
func (c *Content) World(id WorldID) (*WorldDef, bool) { d, ok := c.Worlds[id]; return d, ok }

// TrafficFor returns the traffic configuration of the given system.
func (c *Content) TrafficFor(id SystemID) (*TrafficDef, bool) {
	d, ok := c.Traffic[id]
	return d, ok
}

// GuardFor returns the npc-guard-* definition whose loadout uses the given
// hull — the security-patrol registry (guards are NPCs that are not rats:
// zero bounty, no loot table, spawned by the traffic system rather than
// belt/anomaly spawn groups).
func (c *Content) GuardFor(hull ShipID) (*NpcDef, bool) {
	for _, id := range keys(c.NPCs) {
		if strings.HasPrefix(string(id), "npc-guard-") && c.NPCs[id].Ship == hull {
			return c.NPCs[id], true
		}
	}
	return nil, false
}

// Contract returns the contract def for id.
func (c *Content) Contract(id ContractID) (*ContractDef, bool) {
	d, ok := c.Contracts[id]
	return d, ok
}

// Board returns the contracts offered on the given station's mission board,
// sorted by ID (deterministic order for UI and tests).
func (c *Content) Board(cel CelID) []*ContractDef {
	var out []*ContractDef
	for _, id := range keys(c.Contracts) {
		if c.Contracts[id].Station == cel {
			out = append(out, c.Contracts[id])
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Manufacturing: two-tier recipes (ore -> minerals -> products)
// ---------------------------------------------------------------------------

// RecipeID is the ID of a manufacturing recipe: kebab-case, "rec-" prefix,
// globally unique, frozen once shipped (docs/CONTENT_AUTHORING.md).
type RecipeID string

// Recipe craft-growth families — the station skill-tracking keys. The
// vocabulary stays small on purpose; new families are a schema change, not
// free-form data.
const (
	FamilyHulls   = "hulls"
	FamilyModules = "modules"
	FamilyCharges = "charges"
)

// RecipeInput is one mineral consumed by a recipe run. v1 takes minerals
// only: ore refines into minerals (ItemDef.RefineYield), minerals craft
// into products (RecipeDef).
type RecipeInput struct {
	Item ItemID  `json:"item"`
	Qty  float64 `json:"qty"`
}

// RecipeDef is one manufacturing recipe: minerals in, one product out.
//
// Product resolves the way a market order does — ItemID first, then ShipID
// cast to ItemID, then ModuleID — and only ships, modules and ammo/charge
// items may be products (ores, minerals and loot are never manufactured).
// Qty is units produced per run; Time is seconds per run at craft level 1 (the baseline; craft levels discount it);
// Family is one of the Family* constants above. Loaded from recipes/*.json;
// the schema is a list so multiple recipes per product can exist later even
// though v1 ships exactly one.
type RecipeDef struct {
	ID      RecipeID      `json:"id"`
	Product ItemID        `json:"product"`
	Qty     int           `json:"qty"`
	Inputs  []RecipeInput `json:"inputs"`
	Time    float64       `json:"time"`
	Family  string        `json:"family"`
}

// RecipesInOrder returns every recipe sorted by ID — the canonical iteration
// order for UI, sim and tests.
func (c *Content) RecipesInOrder() []*RecipeDef {
	out := make([]*RecipeDef, 0, len(c.Recipes))
	for _, id := range keys(c.Recipes) {
		out = append(out, c.Recipes[id])
	}
	return out
}
