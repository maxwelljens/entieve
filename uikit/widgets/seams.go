package widgets

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
)

// The three seams (CONTEXT.md: Theme, ContentView, ActionSink). They
// replace the Env god-context: every screen constructor takes exactly
// the subset it uses, so a widget can be built, exercised and asserted
// without a simulation anywhere near it.
//
//	Theme       faces + scaled metrics + palette: pure visual constants
//	ContentView the read-only seam: defs + observable sim state
//	ActionSink  the write seam: Do(Intent), executed by the host
//
// Seams is the union of the three, for the screens that draw, read and
// act; Env is the deprecated name for that union (see its doc below).

// ContentView is the read-only seam: the defs catalog plus the observable
// sim state exactly as screens read it — never the simulation itself. The
// kit-less host and the game both install it over *content.Content and
// *sim.Sim (newView); a test stubs the interface and needs no sim at all.
type ContentView interface {
	// Defs is the content catalog: every def lookup, map and ordered
	// list the screens render from (item/ship/module/recipe/…).
	Defs() *content.Content

	// Player is the player entity (zero if none).
	Player() ecs.Entity

	// Grid is Info for every on-grid entity, deterministic order.
	Grid() []sim.Info

	// Info is the flattened view of one entity.
	Info(ecs.Entity) (sim.Info, bool)

	// System is the current system definition (nil in an empty world).
	System() *content.SystemDef

	// Time is simulated seconds since world start.
	Time() float64

	// Wallet is the player's euro balance.
	Wallet() float64

	// ProficiencyRequirement is a def's proficiency gate as the sim would
	// refuse it ("Requires Autocannon II"; "" when it clears). The one walk
	// over family level and prerequisite edges is the sim's; Show Info
	// prints what it is handed.
	ProficiencyRequirement(family string, gate int) string

	// PlayerDocked reports whether the player is docked somewhere.
	PlayerDocked() bool

	// ServiceAvailable reports whether the player can use a station service
	// HERE: docked, the station offers it and still runs it (a remnant keeps
	// market, repair and refinery and loses industry and fitting). It is the
	// sim's own stationAccess rule, log-free: the enablement every surface
	// gates on, instead of re-deriving it from DockedStation().Services.
	ServiceAvailable(string) bool

	// DockedStation is the docked-at identity (celestial name + system) the
	// station services strip and the scene host read. It carries no service
	// list: what the station HAS lives in the sim's station component, what
	// the player can USE is ServiceAvailable. ok is false while undocked.
	DockedStation() (StationInfo, bool)

	// Component-store reads (observable state, one per store screens
	// touch): a value snapshot of one component — the store's own Get,
	// dereferenced, so nothing hands a screen a live handle to write
	// through. A missing entity yields the zero value and false.
	Fitting(ecs.Entity) (sim.Fitting, bool)
	ShipState(ecs.Entity) (sim.ShipState, bool)
	Motion(ecs.Entity) (sim.Motion, bool)
	Capacitor(ecs.Entity) (sim.Capacitor, bool)
	Cargo(ecs.Entity) (sim.Cargo, bool)
	Targeting(ecs.Entity) (sim.Targeting, bool)
	Asteroid(ecs.Entity) (sim.Asteroid, bool)
	NpcAI(ecs.Entity) (sim.NpcAI, bool)

	// Hangar is the station hangar's contents as one read: the item stacks
	// and the stored hulls, both copies (read-only). The hangar window and
	// the fitting rack read the pair together.
	Hangar() Hangar

	// PredictedAmmo is the charge the next shot from one fitted weapon slot
	// would consume, derived here over the player's bay, cargo and
	// preference so the fitting summary's DPS figure never disagrees with
	// what the gun fires: the bay's loaded round, else its selection, else
	// the CmdSetAmmo preference while it still sits in cargo and matches the
	// gun's ammo class, else the first cargo stack of that class in
	// canonical order. "" = dry, or not a weapon slot.
	PredictedAmmo(sim.SlotRef) content.ItemID

	// ModuleTarget is the entity one active module slot is bound to (the
	// sim's own activation target); ok is false when nothing is bound. The
	// target cards read it to name the modules cycling on them.
	ModuleTarget(ecs.Entity, sim.SlotRef) (ecs.Entity, bool)

	// Clip is one fitted slot's shell bay as the rack reads it — loaded
	// rounds, the bay's whole-round capacity for that round, the selected
	// type and every compatible type in cargo (sorted). ok is false when
	// the slot holds no bay (a charge-free weapon, a non-charge module).
	// Rounds == 0, no Selection and at least one Option is the waiting
	// state: nothing loads until the player picks a type (sim fillClip's
	// clipWaiting case).
	Clip(sim.SlotRef) (sim.ClipState, bool)

	// Market is the market domain's reads as one handle (MarketView): the
	// docked station, one system's book or one station's, the executed
	// quote walk, the cheapest ask anywhere and the displayed ladder.
	Market() MarketView

	// TradeableName is the display name of any marketable id — item,
	// module or hull — falling back to the raw id. The one name read the
	// market, orders, contract board and tooltips share.
	TradeableName(content.ItemID) string

	// Industry is the industry and refinery domain's reads as one handle
	// (IndustryView): the docked station, the GLOBAL job list (every
	// station), and the per-recipe materials, fee, gate, duration and
	// output the manufacturing and refinery panels preview.
	Industry() IndustryView

	// PlayerStock is the player's holdings of one item (every hold
	// summed), the read the order window's sell default and the
	// manufacturing rows use.
	PlayerStock(content.ItemID) int

	// StationName is a station's display name, for the job rows and the
	// contract destinations that name a station the player is not at.
	StationName(sim.StationID) string

	// Holds is the cargo domain's reads as one handle (HoldsView): the
	// general cargo and the dedicated ore hold — the two places CmdRefine
	// and CmdTransfer consume from — and one hold's used/capacity volumes.
	Holds() HoldsView
	// DeliverProgress is a deliver contract's live objective progress in
	// the objective's own unit — m³ for volume items, whole units for
	// volume-less cargo (modules) — capped at the objective's count.
	DeliverProgress(*content.ContractDef) float64

	// MeetsStanding reports whether the player's standing clears a
	// minimum (the sim's contract-accept gate, epsilon included).
	MeetsStanding(float64) bool

	// Contacts is the docked station's contact view: the docked-at
	// celestial, the player's standing, the station's live offers (derived
	// from sim state, canonical order) and every accepted offer. The zero
	// value means undocked (the contact panel renders its empty read).
	Contacts() Contacts

	// Show Info reference resolution (the info catalog seam).
	InfoRefFor(ecs.Entity) (InfoRef, bool)
	InfoRefOf(any) (InfoRef, bool)

	// Finance reads (the Finances window): the wallet's history and the
	// two other pools of the player's money — the capital escrowed in
	// open buy orders and the rewards accepted contracts still owe.
	Ledger() []sim.LedgerEntry
	Deployed() float64
	Receivables() float64

	// War is the war as the screens read it (the starmap's control and
	// contested display). The zero value means the world carries no war.
	War() WarView
}

// WarView is the war as the screens read it (CONTEXT.md: war, front,
// contest, war treasury): which side holds each system, the front's
// contest meters and the Corporate Fleets' treasury — the war's health.
// The maps are copies, never a live handle into the simulation's state,
// and the zero value (nil Control) is a world with no war.
type WarView struct {
	Control  map[content.SystemID]string // WarState.Control: "charter" | "works"
	Contest  map[content.SystemID]ContestView
	Treasury float64
}

// ContestView is one front system's meter: the attacking side's progress
// in [0, Threshold] — the Works' on a charter hold, the charter's on a
// works hold.
type ContestView struct {
	Meter     float64
	Threshold float64
}

// Has reports whether the world carries a war at all.
func (w WarView) Has() bool { return w.Control != nil }

// WorksHeld reports whether the Old Works hold a system. This is the one
// place the UI spells the simulation's works control value.
func (w WarView) WorksHeld(id content.SystemID) bool { return w.Control[id] == "works" }

// StationInfo is one docked station as the interface reads it: the
// identity the station view shows (celestial name + system name). What the
// station HAS is the sim's station component; what the player can USE is
// ServiceAvailable, so this read carries no second copy of the service list.
type StationInfo struct {
	Name   string
	System string
}

// Contacts is the docked station's contact view as one read (CONTEXT.md
// "Station board", "Contact offer"): where the player is docked, the
// standing that gates the contacts, the station's live offers in canonical
// order (derived, never persisted) and every accepted offer with its live
// progress. The zero value means undocked; a panel built on it renders the
// empty view.
type Contacts struct {
	Station  content.CelID
	Standing float64
	Offers   []sim.Offer
	Active   []sim.MissionState
}

// Hangar is the station hangar's contents as one read: the item stacks and
// the stored hulls, both copies (read-only). The zero value is an empty
// hangar (a stub host may hand us a nil one).
type Hangar struct {
	Items []content.CargoStack
	Ships []sim.StoredShip
}

// MarketView is the market domain's reads (CONTEXT.md "Market", "Quote",
// "Price ladder"): one handle, so ContentView carries one member per domain
// instead of one per query. The pricing lives in the sim; only PriceLadder
// derives anything here (the undocked fallback). Nothing is snapshotted:
// each read is callable per row, per frame, with its own arguments.
type MarketView interface {
	// Docked is the station whose book the player trades ("" when not
	// docked at a market).
	Docked() sim.StationID
	// System is one system's resting orders (the undocked catalog).
	System(content.SystemID) []sim.MarketStock
	// Orders is one station's book.
	Orders(sim.StationID) []sim.MarketStock
	// Quote is the sim's executed walk for an amount on one side of a
	// station's book (the wallet-composable read: walk, fee, tax, depth).
	Quote(sim.StationID, content.ItemID, sim.OrderSide, int) sim.Quote
	// CheapestAsk is the cheapest sell order any market station rests for
	// an item.
	CheapestAsk(content.ItemID) (float64, bool)
	// PriceLadder is an item's displayed price: at a station, its book's
	// ladder (last trade, best ask, best bid, base); with station "" the
	// undocked fallback view's ladder over the shown system's orders (best
	// ask, best bid, base — no station book, no last trade). Display only:
	// a trade path prices with Quote.
	PriceLadder(sim.StationID, content.ItemID) float64
}

// IndustryView is the industry and refinery domain's reads (CONTEXT.md
// "Industry", "Job", "Refinery"): the manufacturing and refinery panels
// gate, preview and price a job through it. Jobs are the GLOBAL list (every
// station) — a control gates on Docked.
type IndustryView interface {
	// Docked is the station whose industry the player uses ("").
	Docked() sim.StationID
	// Jobs is every station's manufacturing job, canonical order.
	Jobs() []sim.Job
	// Materials is a job's inputs at runs.
	Materials(sim.StationID, content.RecipeID, int) []content.CargoStack
	// Fee is a job's credit fee at runs.
	Fee(content.RecipeID, int) float64
	// CanStart is the sim's start gate at runs.
	CanStart(sim.StationID, content.RecipeID, int) bool
	// Duration is a job's wall-clock seconds at runs.
	Duration(sim.StationID, content.RecipeID, int) float64
	// Output is the units a job delivers to the player at runs: the sim's
	// per-run output under the Manufacturing edge (RESULT shows it at one
	// run).
	Output(content.RecipeID, int) int
	// Refine is the batch conversion the refinery preview and CmdRefine
	// share, under the Refining edge.
	Refine(*content.ItemDef, int) []content.CargoStack
}

// HoldsView is the cargo domain's reads (CONTEXT.md "Hold"): the two holds
// CmdRefine and CmdTransfer consume from, each as a copy, plus one hold's
// volumes. A screen never holds a live handle into the sim.
type HoldsView interface {
	// Cargo is a copy of the general hold.
	Cargo() []content.CargoStack
	// Ore is a copy of the dedicated ore hold.
	Ore() []content.CargoStack
	// Used is one hold's m³: used volume and capacity, for the hold name
	// CmdTransfer takes ("cargo", "ore"). ok is false when the player has
	// no such hold (no cargo at all, or no ore hold fitted).
	Used(kind string) (used, capacity float64, ok bool)
}

// view is the production ContentView: loaded content plus the live sim,
// exposed read-only. Screens receive it through the seams; nothing on it
// mutates the simulation.
type view struct {
	c *content.Content
	s *sim.Sim
}

// newView wraps a content registry and a simulation as the read-only seam.
func NewView(c *content.Content, s *sim.Sim) ContentView { return &view{c: c, s: s} }

func (v *view) Defs() *content.Content { return v.c }

func (v *view) Player() ecs.Entity { return v.s.Player() }

func (v *view) Grid() []sim.Info { return v.s.Grid() }

func (v *view) Info(e ecs.Entity) (sim.Info, bool) { return v.s.Info(e) }

func (v *view) System() *content.SystemDef { return v.s.System() }

func (v *view) Time() float64 { return v.s.Time() }

func (v *view) Wallet() float64 { return v.s.Wallet() }

func (v *view) ProficiencyRequirement(family string, gate int) string {
	return v.s.ProficiencyRequirement(family, gate)
}

func (v *view) PlayerDocked() bool {
	return v.s.Docked.Has(v.s.Player())
}

// DockedStation resolves the station the player is docked at: identity
// from the system definition (the same read the station label used) and
// the service list from the sim's station component. Undocked → ok false.
func (v *view) DockedStation() (StationInfo, bool) {
	at, ok := v.s.Docked.Get(v.s.Player())
	if !ok {
		return StationInfo{}, false
	}
	var info StationInfo
	if sys := v.s.System(); sys != nil {
		info.System = sys.Name
		for _, cel := range sys.Celestials {
			if cel.ID == at.At {
				info.Name = cel.Name
				break
			}
		}
	}
	return info, true
}

func (v *view) Fitting(e ecs.Entity) (sim.Fitting, bool) {
	c, ok := v.s.Fittings.Get(e)
	if !ok {
		return sim.Fitting{}, false
	}
	return *c, true
}

func (v *view) ShipState(e ecs.Entity) (sim.ShipState, bool) {
	c, ok := v.s.Ships.Get(e)
	if !ok {
		return sim.ShipState{}, false
	}
	return *c, true
}

func (v *view) Motion(e ecs.Entity) (sim.Motion, bool) {
	c, ok := v.s.Motions.Get(e)
	if !ok {
		return sim.Motion{}, false
	}
	return *c, true
}

func (v *view) Capacitor(e ecs.Entity) (sim.Capacitor, bool) {
	c, ok := v.s.Caps.Get(e)
	if !ok {
		return sim.Capacitor{}, false
	}
	return *c, true
}

func (v *view) Cargo(e ecs.Entity) (sim.Cargo, bool) {
	c, ok := v.s.Cargos.Get(e)
	if !ok {
		return sim.Cargo{}, false
	}
	return *c, true
}

func (v *view) Targeting(e ecs.Entity) (sim.Targeting, bool) {
	c, ok := v.s.Targetings.Get(e)
	if !ok {
		return sim.Targeting{}, false
	}
	return *c, true
}

func (v *view) Asteroid(e ecs.Entity) (sim.Asteroid, bool) {
	c, ok := v.s.Asteroids.Get(e)
	if !ok {
		return sim.Asteroid{}, false
	}
	return *c, true
}

func (v *view) NpcAI(e ecs.Entity) (sim.NpcAI, bool) {
	c, ok := v.s.AIs.Get(e)
	if !ok {
		return sim.NpcAI{}, false
	}
	return *c, true
}

// Hangar returns the station hangar's contents. The hangar is allocated
// with the sim, but a stub host may hand us a nil one.
func (v *view) Hangar() Hangar {
	if v.s.Hangar == nil {
		return Hangar{}
	}
	return Hangar{Items: v.s.Hangar.Items, Ships: v.s.Hangar.Ships}
}

// Clip reads one fitted slot's shell bay from the sim, for the player.
func (v *view) Clip(slot sim.SlotRef) (sim.ClipState, bool) {
	return v.s.Clip(v.s.Player(), slot)
}

// ModuleTarget reads one of the player's slots' activation target.
func (v *view) ModuleTarget(e ecs.Entity, slot sim.SlotRef) (ecs.Entity, bool) {
	return v.s.ModuleTarget(e, slot)
}

// PredictedAmmo is the charge the gun would fire, off the sim's own bay read:
// the round in the bay, else the type its standing order names. No order means
// the gun fires nothing (""), whatever cargo holds — the sim loads only a
// chosen type.
func (v *view) PredictedAmmo(slot sim.SlotRef) content.ItemID {
	cs, ok := v.Clip(slot)
	if !ok {
		return ""
	}
	if cs.Item != "" {
		return cs.Item
	}
	return cs.Selection
}

func (v *view) Market() MarketView { return marketView{v} }

// ServiceAvailable forwards the sim's one access rule: docked + offered +
// not masked (a remnant's charter services are dark).
func (v *view) ServiceAvailable(svc string) bool { return v.s.ServiceAvailable(svc) }

// marketView is the market domain over one view: every read forwards the
// sim's own answer, except PriceLadder's undocked fallback, which has no
// single book to forward.
type marketView struct{ v *view }

func (m marketView) Docked() sim.StationID { return m.v.s.DockedMarket() }

func (m marketView) System(id content.SystemID) []sim.MarketStock { return m.v.s.Market(id) }

func (m marketView) Orders(st sim.StationID) []sim.MarketStock { return m.v.s.BookOrders(st) }

// Quote forwards the sim's executed walk: no pricing lives here.
func (m marketView) Quote(st sim.StationID, item content.ItemID, side sim.OrderSide, qty int) sim.Quote {
	return m.v.s.Quote(st, item, side, qty)
}

func (m marketView) CheapestAsk(item content.ItemID) (float64, bool) { return m.v.s.CheapestAsk(item) }

// PriceLadder: a station's book carries sim's full ladder (last trade,
// ask, bid, base); the undocked view has no single book, so it walks the
// shown system's orders for the ask/bid rungs and lets the base rung come
// from the same sim read.
func (m marketView) PriceLadder(station sim.StationID, item content.ItemID) float64 {
	if station != "" {
		return m.v.s.MarketPrice(station, item)
	}
	ask, bid := -1.0, -1.0
	if sys := m.v.s.System(); sys != nil {
		// shortcut: the undocked quote re-copies the system book per
		// item; memoise per frame if the catalog grows into the hundreds.
		for _, o := range m.v.s.Market(sys.ID) {
			if o.Item != item {
				continue
			}
			if o.Side == sim.OrderSell {
				if ask < 0 || o.Price < ask {
					ask = o.Price
				}
			} else if o.Price > bid {
				bid = o.Price
			}
		}
	}
	if ask >= 0 {
		return ask
	}
	if bid >= 0 {
		return bid
	}
	return m.v.s.MarketPrice("", item) // base price
}

// TradeableName mirrors the sim's own itemName (items, modules, hulls) so
// the UI names an id exactly as the log lines do.
func (v *view) TradeableName(id content.ItemID) string {
	return TradeableName(v.c, id)
}

func (v *view) Industry() IndustryView { return industryView{v} }

func (v *view) StationName(st sim.StationID) string { return v.s.StationName(st) }

func (v *view) PlayerStock(item content.ItemID) int { return v.s.PlayerStock(item) }

func (v *view) Holds() HoldsView { return holdsView{v} }

// industryView is the industry domain over one view: every read forwards
// the sim's own answer (its edges and gates), nothing is derived here.
type industryView struct{ v *view }

func (i industryView) Docked() sim.StationID { return i.v.s.DockedIndustry() }

func (i industryView) Jobs() []sim.Job { return i.v.s.AllJobs() }

func (i industryView) Materials(st sim.StationID, rec content.RecipeID, runs int) []content.CargoStack {
	return i.v.s.JobMaterials(st, rec, runs)
}

func (i industryView) Fee(rec content.RecipeID, runs int) float64 { return i.v.s.JobFee(rec, runs) }

func (i industryView) CanStart(st sim.StationID, rec content.RecipeID, runs int) bool {
	return i.v.s.CanStartJob(st, rec, runs)
}

func (i industryView) Duration(st sim.StationID, rec content.RecipeID, runs int) float64 {
	return i.v.s.JobDuration(st, rec, runs)
}

func (i industryView) Output(rec content.RecipeID, runs int) int {
	return i.v.s.PlayerJobOutput(rec, runs)
}

func (i industryView) Refine(def *content.ItemDef, qty int) []content.CargoStack {
	return i.v.s.RefineOutput(def, qty)
}

// holdsView is the cargo domain over one view: copies of the two holds and
// the hold-volume rule.
type holdsView struct{ v *view }

func (h holdsView) Cargo() []content.CargoStack {
	if c, ok := h.v.s.Cargos.Get(h.v.s.Player()); ok {
		return append([]content.CargoStack(nil), c.Stacks...)
	}
	return nil
}

func (h holdsView) Ore() []content.CargoStack {
	if c, ok := h.v.s.Cargos.Get(h.v.s.Player()); ok {
		return append([]content.CargoStack(nil), c.Ore...)
	}
	return nil
}

// Used is the hold-volume rule once: used m³ from the item defs (unknown
// and volume-less ids occupy nothing, the sim's itemVolume rule) and the
// capacity the hull built.
func (h holdsView) Used(kind string) (used, capacity float64, ok bool) {
	c, ok := h.v.s.Cargos.Get(h.v.s.Player())
	if !ok {
		return 0, 0, false
	}
	var st []content.CargoStack
	switch kind {
	case "cargo":
		st, capacity = c.Stacks, c.Capacity
	case "ore":
		if c.OreCapacity <= 0 {
			return 0, 0, false
		}
		st, capacity = c.Ore, c.OreCapacity
	default:
		return 0, 0, false
	}
	for _, s := range st {
		if d, ok := h.v.c.Item(s.Item); ok && d.Volume > 0 {
			used += d.Volume * float64(s.Qty)
		}
	}
	return used, capacity, true
}

// DeliverProgress counts the objective item in both holds — the two places
// the sim consumes a deliver objective from — in the unit the contract's
// count is written in: m³ for a volume item, whole units for a module.
func (v *view) DeliverProgress(def *content.ContractDef) float64 {
	if def == nil {
		return 0
	}
	qty := 0
	if c, ok := v.s.Cargos.Get(v.s.Player()); ok {
		for _, st := range c.Stacks {
			if st.Item == def.Objective.Item {
				qty += st.Qty
			}
		}
		for _, st := range c.Ore {
			if st.Item == def.Objective.Item {
				qty += st.Qty
			}
		}
	}
	need := float64(def.Objective.Count)
	var have float64
	if d, ok := v.c.Item(def.Objective.Item); ok && d.Volume > 0 {
		have = float64(qty) * d.Volume
	} else {
		have = float64(qty)
	}
	if have > need {
		have = need
	}
	return have
}

// MeetsStanding reads the sim's accept gate; the contact row and the
// dialog both call it, so neither re-derives the epsilon.
func (v *view) MeetsStanding(min float64) bool { return v.s.MeetsStanding(min) }

// Contacts is the contact view read once: the docked celestial, the standing
// (a plain read; the gate itself is sim.MeetsStanding), the station's
// derived offers and the accepted missions. Undocked → the zero value.
func (v *view) Contacts() Contacts {
	at, ok := v.s.Docked.Get(v.s.Player())
	if !ok {
		return Contacts{}
	}
	return Contacts{
		Station:  at.At,
		Standing: v.s.Standing(),
		Offers:   v.s.ContactOffers(at.At),
		Active:   v.s.Missions(),
	}
}

func (v *view) InfoRefFor(e ecs.Entity) (InfoRef, bool) { return InfoRefFor(v, e) }

func (v *view) InfoRefOf(data any) (InfoRef, bool) { return InfoRefOf(v, data) }

func (v *view) Ledger() []sim.LedgerEntry { return v.s.Ledger() }

func (v *view) Deployed() float64 { return v.s.Deployed() }

func (v *view) Receivables() float64 { return v.s.Receivables() }

// War copies the war state: control, the front's meters and the treasury.
// A world without a war reads as the zero WarView (nil maps).
func (v *view) War() WarView {
	st := v.s.War()
	if st == nil {
		return WarView{}
	}
	w := WarView{
		Control:  make(map[content.SystemID]string, len(st.Control)),
		Contest:  make(map[content.SystemID]ContestView, len(st.Contest)),
		Treasury: st.Treasury,
	}
	for id, side := range st.Control {
		w.Control[id] = side
	}
	for id, c := range st.Contest {
		if c != nil {
			w.Contest[id] = ContestView{Meter: c.Meter, Threshold: c.Threshold}
		}
	}
	return w
}

// ActionSink is the write seam: a screen expresses "do this" as a typed
// Intent (uikit/widgets/intents.go) and never touches the simulation. The host
// installs the executor — the game maps every intent onto the handler it
// dispatches to (Game.Do); a kit-less host runs uikit's built-in one over
// its enqueue callback (exec, uikit/widgets/intents.go).
type ActionSink interface {
	Do(Intent)
}

// sinkFunc adapts a function to ActionSink: the kit's panels hold one of
// these so they always reach the sink the host installed last, no matter
// when they were built.
type SinkFunc func(Intent)

func (f SinkFunc) Do(i Intent) {
	if f != nil {
		f(i)
	}
}

// Seams is the union of the three seams — the constructor param of every
// screen that draws, reads state AND acts (a market table, the Show Info
// window, the module rack). Screens that need less declare less: a
// Theme-only widget takes Theme, a reader takes Theme+ContentView.
//
// It also carries the host's pointer frame (CONTEXT.md "Pointer frame"):
// one per host, created by the game and shared by every panel, so a frame
// has exactly one sample and one cursor shape whoever polls it.
type Seams interface {
	theme.Theme
	ContentView
	ActionSink

	// Pointer is the host's pointer frame. Widgets built from these seams
	// read their input from it instead of the OS.
	Pointer() *Pointer
}

// seams carries the three seams as one assignable value (struct
// embedding: the union method set, no god-bag behind it).
type seams struct {
	theme.Theme
	ContentView
	ActionSink
	ptr *Pointer
}

// Pointer is the host's pointer frame (see Seams).
func (s seams) Pointer() *Pointer { return s.ptr }

// NewSeams packages the three seams as one assignable value — what
// hosts outside widgets (the session builder, the kit) construct the
// union through.
func NewSeams(t theme.Theme, cv ContentView, s ActionSink, p *Pointer) Seams {
	if p == nil {
		p = NewPointer(nil)
	}
	return seams{Theme: t, ContentView: cv, ActionSink: s, ptr: p}
}

// Env is the deprecated name for Seams: the god-context reduced to the
// union of the three seams. It is kept as an alias for the signatures
// that still read as a bundle (uikit's test helpers); it carries no
// state of its own — the Content/Sim/Theme/Pal/TS/Enqueue/faces fields
// and the services are gone. Prefer naming the seam a screen needs.
type Env = Seams

// Panel is the common shape of every chrome panel: a widget tree plus Sync,
// which refreshes displayed state from the sim (called once per frame).
type Panel interface {
	Sync()
}

// WindowPanel is a Panel that owns a draggable/resizable ebitenui window.
type WindowPanel interface {
	Panel
	Win() *widget.Window
}
