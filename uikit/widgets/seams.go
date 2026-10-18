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

	// PlayerDocked reports whether the player is docked somewhere.
	PlayerDocked() bool

	// DockedStation is the docked-at identity plus the services that
	// station offers ("market", "fitting", "industry", "repair",
	// "refine") — the reads behind the station services view's identity
	// line and per-service buttons, and the rail's gray-out of
	// station-dependent entries. ok is false while undocked.
	DockedStation() (StationInfo, bool)

	// Component-store reads (observable state, one per store screens
	// touch): a value snapshot of one component — the store's own Get,
	// dereferenced, so nothing hands a screen a live handle to write
	// through. A missing entity yields the zero value and false.
	Transform(ecs.Entity) (sim.Transform, bool)
	Fitting(ecs.Entity) (sim.Fitting, bool)
	ShipState(ecs.Entity) (sim.ShipState, bool)
	Motion(ecs.Entity) (sim.Motion, bool)
	Capacitor(ecs.Entity) (sim.Capacitor, bool)
	Cargo(ecs.Entity) (sim.Cargo, bool)
	Targeting(ecs.Entity) (sim.Targeting, bool)
	Asteroid(ecs.Entity) (sim.Asteroid, bool)
	NpcAI(ecs.Entity) (sim.NpcAI, bool)

	// Hangar is the station hangar's contents (copies: read-only).
	HangarItems() []content.CargoStack
	HangarShips() []sim.StoredShip

	// Market books and prices.
	Market(content.SystemID) []sim.MarketStock
	BookOrders(sim.StationID) []sim.MarketStock
	MarketPrice(sim.StationID, content.ItemID) float64
	DockedMarket() sim.StationID

	// Industry: the factory's jobs, materials, gating and craft reads.
	DockedIndustry() sim.StationID
	Jobs(sim.StationID) []sim.Job
	JobMaterials(sim.StationID, content.RecipeID, int) []content.CargoStack
	CanStartJob(sim.StationID, content.RecipeID, int) bool
	PlayerStock(content.ItemID) int
	JobDuration(sim.StationID, content.RecipeID, int) float64
	CraftLevel(sim.StationID, string) int

	// Show Info reference resolution (the info catalog seam).
	InfoRefFor(ecs.Entity) (InfoRef, bool)
	InfoRefOf(any) (InfoRef, bool)

	// Finance reads (the Finances window): the wallet's history and the
	// two other pools of the player's money — the capital escrowed in
	// open buy orders and the rewards accepted contracts still owe.
	Ledger() []sim.LedgerEntry
	Deployed() float64
	Receivables() float64
}

// StationInfo is one docked station as the interface reads it: the
// identity the station view shows (celestial name + system name) and the
// service names it offers. Services is a copy — a screen never holds a
// live handle into the sim.
type StationInfo struct {
	Name     string
	System   string
	Services []string
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
	// The station's entity is the celestial with this CelID: the sim's
	// own dockedStation read walks the same pair (celEntity is private).
	var services []string
	v.s.Stations.Each(func(e ecs.Entity, st *sim.Station) {
		if c, ok := v.s.Celestials.Get(e); ok && c.Cel == at.At && st != nil {
			services = append([]string(nil), st.Services...)
		}
	})
	info.Services = services
	return info, true
}

func (v *view) Transform(e ecs.Entity) (sim.Transform, bool) {
	c, ok := v.s.Transforms.Get(e)
	if !ok {
		return sim.Transform{}, false
	}
	return *c, true
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

// HangarItems returns the station hangar's item stacks. The hangar is
// allocated with the sim, but a stub host may hand us a nil one.
func (v *view) HangarItems() []content.CargoStack {
	if v.s.Hangar == nil {
		return nil
	}
	return v.s.Hangar.Items
}

func (v *view) HangarShips() []sim.StoredShip {
	if v.s.Hangar == nil {
		return nil
	}
	return v.s.Hangar.Ships
}

func (v *view) Market(id content.SystemID) []sim.MarketStock { return v.s.Market(id) }

func (v *view) BookOrders(st sim.StationID) []sim.MarketStock { return v.s.BookOrders(st) }

func (v *view) MarketPrice(st sim.StationID, item content.ItemID) float64 {
	return v.s.MarketPrice(st, item)
}

func (v *view) DockedMarket() sim.StationID { return v.s.DockedMarket() }

func (v *view) DockedIndustry() sim.StationID { return v.s.DockedIndustry() }

func (v *view) Jobs(st sim.StationID) []sim.Job { return v.s.Jobs(st) }

func (v *view) JobMaterials(st sim.StationID, rec content.RecipeID, runs int) []content.CargoStack {
	return v.s.JobMaterials(st, rec, runs)
}

func (v *view) CanStartJob(st sim.StationID, rec content.RecipeID, runs int) bool {
	return v.s.CanStartJob(st, rec, runs)
}

func (v *view) PlayerStock(item content.ItemID) int { return v.s.PlayerStock(item) }

func (v *view) JobDuration(st sim.StationID, rec content.RecipeID, runs int) float64 {
	return v.s.JobDuration(st, rec, runs)
}

func (v *view) CraftLevel(st sim.StationID, family string) int { return v.s.CraftLevel(st, family) }

func (v *view) InfoRefFor(e ecs.Entity) (InfoRef, bool) { return InfoRefFor(v, e) }

func (v *view) InfoRefOf(data any) (InfoRef, bool) { return InfoRefOf(v, data) }

func (v *view) Ledger() []sim.LedgerEntry { return v.s.Ledger() }

func (v *view) Deployed() float64 { return v.s.Deployed() }

func (v *view) Receivables() float64 { return v.s.Receivables() }

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
type Seams interface {
	theme.Theme
	ContentView
	ActionSink
}

// seams carries the three seams as one assignable value (struct
// embedding: the union method set, no god-bag behind it).
type seams struct {
	theme.Theme
	ContentView
	ActionSink
}

// NewSeams packages the three seams as one assignable value — what
// hosts outside widgets (the session builder, the kit) construct the
// union through.
func NewSeams(t theme.Theme, cv ContentView, s ActionSink) Seams {
	return seams{Theme: t, ContentView: cv, ActionSink: s}
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
