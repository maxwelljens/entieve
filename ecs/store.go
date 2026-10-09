package ecs

// Store holds one component type T per entity, in insertion order.
//
// Storage is a map from entity to value plus a slice recording insertion
// order, which makes iteration deterministic. Removing an entity preserves
// the relative order of the rest; re-inserting an entity moves it to the end.
// A Store is not safe for concurrent use.
type Store[T any] struct {
	w     *World
	m     map[Entity]*T
	order []Entity
}

// NewStore creates a Store on w and registers it so that World.Destroy
// removes the destroyed entity's component automatically.
func NewStore[T any](w *World) *Store[T] {
	s := &Store[T]{w: w, m: make(map[Entity]*T)}
	w.register(func(e Entity) { s.Remove(e) })
	return s
}

// Set inserts or replaces the component for e. It is a no-op if e is not
// alive in the world.
func (s *Store[T]) Set(e Entity, v T) {
	if !s.w.Alive(e) {
		return
	}
	if p, ok := s.m[e]; ok {
		*p = v
		return
	}
	p := new(T)
	*p = v
	s.m[e] = p
	s.order = append(s.order, e)
}

// Get returns a pointer into the store for in-place mutation, or ok == false
// if e has no component here.
func (s *Store[T]) Get(e Entity) (*T, bool) {
	p, ok := s.m[e]
	return p, ok
}

// Has reports whether e has a component in this store.
func (s *Store[T]) Has(e Entity) bool {
	_, ok := s.m[e]
	return ok
}

// Remove deletes e's component, preserving the order of the remaining
// entities. Removing an entity that is not present is a no-op.
func (s *Store[T]) Remove(e Entity) {
	if _, ok := s.m[e]; !ok {
		return
	}
	delete(s.m, e)
	for i, o := range s.order {
		if o == e {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

// Len returns the number of components in this store.
func (s *Store[T]) Len() int { return len(s.m) }

// Entities returns a snapshot of the stored entities in insertion order.
func (s *Store[T]) Entities() []Entity {
	return append([]Entity(nil), s.order...)
}

// Each calls fn for every entity in insertion order. It iterates a snapshot
// of the order, so it is safe to Create, Destroy, Set, or Remove any entity
// (including the one being visited) inside fn; entities removed mid-pass are
// skipped, and entities first inserted mid-pass are not visited in that pass.
func (s *Store[T]) Each(fn func(e Entity, v *T)) {
	snapshot := append([]Entity(nil), s.order...)
	for _, e := range snapshot {
		if p, ok := s.m[e]; ok {
			fn(e, p)
		}
	}
}
