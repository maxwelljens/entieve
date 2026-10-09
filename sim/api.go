package sim

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// Sim is the deterministic Entieve simulation.
//
// The exported stores are the render/UI read surface; mutate game state only
// through Queue(Command) so determinism and replays hold.
//
// Implementation note: the method bodies in this file are stubs owned by the
// sim implementation workstream. The signatures are a binding contract —
// other packages (render, ui, game, tests) compile against them and must not
// require changes.
type Sim struct {
	Content *content.Content
	World   *ecs.World
	RNG     *core.RNG
	Hangar  *Hangar

	// Component stores (read surface).
	Names      *ecs.Store[Name]
	Transforms *ecs.Store[Transform]
	Intents    *ecs.Store[Intent]
	Motions    *ecs.Store[Motion]
	Ships      *ecs.Store[ShipState]
	Caps       *ecs.Store[Capacitor]
	Cargos     *ecs.Store[Cargo]
	Fittings   *ecs.Store[Fitting]
	Targetings *ecs.Store[Targeting]
	Pilots     *ecs.Store[Pilot]
	AIs        *ecs.Store[NpcAI]
	Asteroids  *ecs.Store[Asteroid]
	Celestials *ecs.Store[Celestial]
	Gates      *ecs.Store[Stargate]
	Stations   *ecs.Store[Station]
	Docked     *ecs.Store[Docked]
	Warps      *ecs.Store[WarpDrive]
	Wrecks     *ecs.Store[Wreck]
}

// New builds a simulation from loaded content and a world scenario, placing
// the player at the world's start and populating the starting system.
func New(c *content.Content, w *content.WorldDef, seed int64) (*Sim, error) {
	panic("sim: not implemented")
}

// Step advances the simulation by one tick (DT seconds). Queued commands are
// consumed first, then systems run in documented order (docs/ARCHITECTURE.md).
func (s *Sim) Step() { panic("sim: not implemented") }

// Queue enqueues a player/agent command for the next Step.
func (s *Sim) Queue(cmd Command) { panic("sim: not implemented") }

// Events drains and returns all pending events.
func (s *Sim) Events() []Event { panic("sim: not implemented") }

// Time returns simulated seconds since world start.
func (s *Sim) Time() float64 { panic("sim: not implemented") }

// Ticks returns the number of executed Steps.
func (s *Sim) Ticks() int64 { panic("sim: not implemented") }

// Hash returns a deterministic fingerprint of full simulation state.
func (s *Sim) Hash() uint64 { panic("sim: not implemented") }

// Player returns the player entity (zero if none).
func (s *Sim) Player() ecs.Entity { panic("sim: not implemented") }

// System returns the current system definition.
func (s *Sim) System() *content.SystemDef { panic("sim: not implemented") }

// Wallet returns the player's credit balance.
func (s *Sim) Wallet() float64 { panic("sim: not implemented") }

// LoadSystem (re)builds the grid for the given system, placing the player
// near the given celestial (gate arrivals, undock, world start).
func (s *Sim) LoadSystem(id content.SystemID, entryAtCel content.CelID) error {
	panic("sim: not implemented")
}

// Grid returns Info for every on-grid entity (docked entities excluded),
// deterministic order.
func (s *Sim) Grid() []Info { panic("sim: not implemented") }

// Info returns the flattened view of one entity.
func (s *Sim) Info(e ecs.Entity) (Info, bool) { panic("sim: not implemented") }

// Snapshot captures full serializable state.
func (s *Sim) Snapshot() (*Save, error) { panic("sim: not implemented") }

// Restore replaces simulation state from a snapshot.
func (s *Sim) Restore(save *Save) error { panic("sim: not implemented") }
