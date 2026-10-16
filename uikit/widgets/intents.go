package widgets

import (
	"image"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// Typed intents (CONTEXT.md: ActionSink). A screen never says HOW: it
// emits an Intent and the host's ActionSink executes it — the game maps
// each one onto the handler it dispatches to (Game.Do → Kit.RunIntent),
// and later the AI surface will emit the very same intents.
//
// Intents are data only: no behaviour, no sim handles. Where a
// sim.Command exists the intent mirrors it (the Cmd prefix is the only
// name it drops — Buy dispatches to CmdBuy, StartJob to CmdStartJob, …),
// with two documented exceptions: WarpTo/Orbit read their Range from the
// Selected Item selectors at press time (so Range is not on the intent),
// and the world actions carry Entity instead of Target.

// Intent is one "do this" a screen expresses. Like sim.Command it is a
// closed set of data types behind a marker method.
type Intent interface{ isIntent() }

type (
	// ShowInfo opens the Show Info window on Ref (the Show Info menu
	// entries, the double-click gesture and every info link).
	ShowInfo struct{ Ref InfoRef }

	// OpenMenu mounts the context menu the row installer derived, at the
	// click position — the popup itself rides the sink.
	OpenMenu struct {
		Entries []MenuEntry
		At      image.Point
	}

	// Buy/Sell trade at the docked station's market.
	Buy struct {
		Item content.ItemID
		Qty  int
	}
	Sell struct {
		Item content.ItemID
		Qty  int
	}

	// Transfer moves item units between holds ("cargo", "ore", "hangar").
	Transfer struct {
		Item content.ItemID
		Qty  int
		From string
		To   string
	}

	// Fit/Unequip/Assemble are the fitting window's three verbs.
	Fit struct {
		Module content.ModuleID
		Slot   sim.SlotRef
	}
	Unequip  struct{ Slot sim.SlotRef }
	Assemble struct{ Ship content.ShipID }

	// StartJob/CollectJob are the factory's two verbs.
	StartJob struct {
		Recipe content.RecipeID
		Runs   int
	}
	CollectJob struct{ Job uint64 }

	// Accept/Abandon are the station contract board's two verbs.
	Accept  struct{ Contract content.ContractID }
	Abandon struct{ Contract content.ContractID }

	// Activate/Deactivate/CycleCharge are the module rack's three verbs:
	// click (toggle), and the cell menu's charge cycle.
	Activate struct {
		Slot   sim.SlotRef
		Target ecs.Entity
	}
	Deactivate  struct{ Slot sim.SlotRef }
	CycleCharge struct {
		Slot sim.SlotRef
		Item content.ItemID
	}

	// WarpTo/Orbit/Approach/Lock/Stop/Dock/Jump/Loot/Mine are the world
	// actions the Selected Item panel and the overview rows offer. Range
	// (warp, orbit) is read from the selectors when the intent executes;
	// Mine carries the laser slot the panel resolved at press.
	WarpTo   struct{ Entity ecs.Entity }
	Orbit    struct{ Entity ecs.Entity }
	Approach struct{ Entity ecs.Entity }
	Lock     struct{ Entity ecs.Entity }
	Stop     struct{}
	Dock     struct{ Entity ecs.Entity }
	Jump     struct{ Entity ecs.Entity }
	Loot     struct{ Entity ecs.Entity }
	Mine     struct {
		Entity ecs.Entity
		Slot   sim.SlotRef
	}

	// Toggle opens/closes the Screen spec row titled Screen (the spec's
	// Title, case-insensitive).
	Toggle struct{ Screen string }

	// Undock leaves the current station; Save/Load/Quit are the Esc
	// menu's verbs. The game executes these itself.
	Undock struct{}
	Save   struct{}
	Load   struct{}
	Quit   struct{}
)

func (ShowInfo) isIntent()    {}
func (OpenMenu) isIntent()    {}
func (Buy) isIntent()         {}
func (Sell) isIntent()        {}
func (Transfer) isIntent()    {}
func (Fit) isIntent()         {}
func (Unequip) isIntent()     {}
func (Assemble) isIntent()    {}
func (StartJob) isIntent()    {}
func (CollectJob) isIntent()  {}
func (Accept) isIntent()      {}
func (Abandon) isIntent()     {}
func (Activate) isIntent()    {}
func (Deactivate) isIntent()  {}
func (CycleCharge) isIntent() {}
func (WarpTo) isIntent()      {}
func (Orbit) isIntent()       {}
func (Approach) isIntent()    {}
func (Lock) isIntent()        {}
func (Stop) isIntent()        {}
func (Dock) isIntent()        {}
func (Jump) isIntent()        {}
func (Loot) isIntent()        {}
func (Mine) isIntent()        {}
func (Toggle) isIntent()      {}
func (Undock) isIntent()      {}
func (Save) isIntent()        {}
func (Load) isIntent()        {}
func (Quit) isIntent()        {}

// commandFor maps an intent onto the sim command its handler queues —
// the ONE intent→command table, shared by uikit's built-in executor and
// the game's. warpRange/orbitRange are the Selected Item selectors'
// current values, read at press (0 where there is no selector, e.g. a
// kit-less session). ok is false for the intents a host handles itself:
// ShowInfo, OpenMenu, Toggle, Save, Load, Quit.
func commandFor(i Intent, warpRange, orbitRange float64) (sim.Command, bool) {
	switch v := i.(type) {
	case ShowInfo, OpenMenu, Toggle, Save, Load, Quit:
		return nil, false
	case Buy:
		return sim.CmdBuy{Item: v.Item, Qty: v.Qty}, true
	case Sell:
		return sim.CmdSell{Item: v.Item, Qty: v.Qty}, true
	case Transfer:
		return sim.CmdTransfer{Item: v.Item, Qty: v.Qty, From: v.From, To: v.To}, true
	case Fit:
		return sim.CmdFitModule{Item: v.Module, Slot: v.Slot}, true
	case Unequip:
		return sim.CmdUnfitModule{Slot: v.Slot}, true
	case Assemble:
		return sim.CmdAssemble{Ship: v.Ship}, true
	case StartJob:
		return sim.CmdStartJob{Recipe: v.Recipe, Runs: v.Runs}, true
	case CollectJob:
		return sim.CmdCollectJob{ID: v.Job}, true
	case Accept:
		return sim.CmdAcceptMission{ID: v.Contract}, true
	case Abandon:
		return sim.CmdAbandonMission{ID: v.Contract}, true
	case Activate:
		return sim.CmdActivate{Slot: v.Slot, Target: v.Target}, true
	case Deactivate:
		return sim.CmdDeactivate{Slot: v.Slot}, true
	case CycleCharge:
		return sim.CmdSetAmmo{Slot: v.Slot, Item: v.Item}, true
	case WarpTo:
		return sim.CmdWarpTo{Target: v.Entity, Range: warpRange}, true
	case Orbit:
		return sim.CmdOrbit{Target: v.Entity, Range: orbitRange}, true
	case Approach:
		return sim.CmdApproach{Target: v.Entity}, true
	case Lock:
		return sim.CmdLock{Target: v.Entity}, true
	case Stop:
		return sim.CmdStop{}, true
	case Dock:
		return sim.CmdDock{Station: v.Entity}, true
	case Jump:
		return sim.CmdJump{Gate: v.Entity}, true
	case Loot:
		return sim.CmdLoot{Wreck: v.Entity}, true
	case Mine:
		return sim.CmdMine{Asteroid: v.Entity, Slot: v.Slot}, true
	case Undock:
		return sim.CmdUndock{}, true
	}
	return nil, false
}

// exec is uikit's built-in ActionSink: the session intents (Show Info,
// the context menu) run on the services its host owns, everything else
// rides commandFor onto the queue tail. The game installs its own sink
// on top (Kit.SetSink); Kit.RunIntent is this one.
type Exec struct {
	show   func(InfoRef)
	open   func([]MenuEntry, image.Point)
	queue  func(sim.Command)
	ranges func() (warp, orbit float64)
}

func (x *Exec) Do(i Intent) {
	switch v := i.(type) {
	case ShowInfo:
		if x.show != nil {
			x.show(v.Ref)
		}
	case OpenMenu:
		if x.open != nil {
			x.open(v.Entries, v.At)
		}
	default:
		var warp, orbit float64
		if x.ranges != nil {
			warp, orbit = x.ranges()
		}
		if cmd, ok := commandFor(i, warp, orbit); ok && x.queue != nil {
			x.queue(cmd)
		}
	}
}

// NewExec builds the built-in executor over the host's callbacks: show
// and open run the session services, queue tails every command intent
// to the host's enqueue, ranges reads the selectors' current warp /
// orbit at dispatch (nil = zeros). This is what the kit and a bare
// NewEnv install as their starting ActionSink.
func NewExec(show func(InfoRef), open func([]MenuEntry, image.Point), queue func(sim.Command), ranges func() (warp, orbit float64)) *Exec {
	return &Exec{show: show, open: open, queue: queue, ranges: ranges}
}
