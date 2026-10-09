// Package ecs is a minimal deterministic entity-component-system.
// Iteration order is deterministic (component insertion order) — simulation
// correctness and replay depend on it.
package ecs

// Entity is a handle to a world entity: the high 32 bits hold the generation,
// the low 32 bits the slot index. The zero value means "no entity".
type Entity uint64

// Index returns the slot index of the entity.
func (e Entity) Index() uint32 { return uint32(e) }

// Generation returns the generation of the entity. A slot reuses its index
// for new entities with a bumped generation, so stale handles never match a
// live entity.
func (e Entity) Generation() uint32 { return uint32(e >> 32) }

// IsZero reports whether e is the zero value, i.e. "no entity".
func (e Entity) IsZero() bool { return e == 0 }

// slot is the per-index bookkeeping of the world's entity table.
type slot struct {
	generation uint32
	alive      bool
}

// World owns entities and the component stores registered against it.
// Destroying an entity removes it from every store created on the world.
// A World is not safe for concurrent use.
type World struct {
	slots    []slot
	free     []uint32
	live     int
	cleanups []func(e Entity)
}

// New returns an empty World.
func New() *World { return &World{} }

// Create returns a new alive Entity, reusing a free slot when one exists.
// Reused slots get a bumped generation, so previously handed-out handles for
// that slot report !Alive.
func (w *World) Create() Entity {
	if n := len(w.free); n > 0 {
		i := w.free[n-1]
		w.free = w.free[:n-1]
		s := &w.slots[i]
		s.generation++
		if s.generation == 0 {
			// Generation never wraps to 0: a handle with generation 0 at
			// index 0 would be indistinguishable from the zero Entity.
			s.generation = 1
		}
		s.alive = true
		w.live++
		return Entity(uint64(s.generation)<<32 | uint64(i))
	}
	const firstGeneration = uint32(1)
	w.slots = append(w.slots, slot{generation: firstGeneration, alive: true})
	w.live++
	return Entity(uint64(firstGeneration)<<32 | uint64(len(w.slots)-1))
}

// Destroy marks e dead, removes it from every registered store, and frees its
// slot for reuse. Destroying a dead, stale, unknown, or zero entity is a
// no-op, so it is always safe to call.
func (w *World) Destroy(e Entity) {
	if !w.Alive(e) {
		return
	}
	for _, cleanup := range w.cleanups {
		cleanup(e)
	}
	s := &w.slots[e.Index()]
	s.alive = false
	w.free = append(w.free, e.Index())
	w.live--
}

// Alive reports whether e refers to a currently live entity of this world.
func (w *World) Alive(e Entity) bool {
	if e.IsZero() || int(e.Index()) >= len(w.slots) {
		return false
	}
	s := &w.slots[e.Index()]
	return s.alive && s.generation == e.Generation()
}

// Len returns the number of live entities.
func (w *World) Len() int { return w.live }

// register adds a cleanup invoked on Destroy for the entity, used by stores.
func (w *World) register(cleanup func(e Entity)) {
	w.cleanups = append(w.cleanups, cleanup)
}
