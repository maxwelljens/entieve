package ecs

import "testing"

func entitiesEqual(a, b []Entity) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStoreSetGetHasRemove(t *testing.T) {
	w := New()
	s := NewStore[string](w)
	e1, e2 := w.Create(), w.Create()

	s.Set(e1, "a")
	s.Set(e2, "b")
	if s.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", s.Len())
	}
	if !s.Has(e1) || !s.Has(e2) {
		t.Error("Has missing a stored entity")
	}
	if v, ok := s.Get(e1); !ok || *v != "a" {
		t.Errorf("Get(e1) = (%v, %v), want (a, true)", v, ok)
	}

	// Replace keeps length and order.
	s.Set(e1, "a2")
	if s.Len() != 2 {
		t.Errorf("replace changed Len() to %d, want 2", s.Len())
	}
	if v, _ := s.Get(e1); *v != "a2" {
		t.Errorf("Get(e1) after replace = %v, want a2", *v)
	}
	if got := s.Entities(); !entitiesEqual(got, []Entity{e1, e2}) {
		t.Errorf("Entities() = %v, want [e1 e2]", got)
	}

	// Pointer returned by Get mutates the stored value in place.
	p, ok := s.Get(e1)
	if !ok {
		t.Fatal("Get(e1) not found")
	}
	*p = "mutated"
	if v, _ := s.Get(e1); *v != "mutated" {
		t.Errorf("in-place mutation lost, got %v", *v)
	}

	// Remove drops just that entity and preserves remaining order.
	s.Remove(e1)
	if s.Has(e1) {
		t.Error("Has true after Remove")
	}
	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}
	if got := s.Entities(); !entitiesEqual(got, []Entity{e2}) {
		t.Errorf("Entities() = %v, want [e2]", got)
	}

	// Removing again, or fetching a removed entity, is a safe no-op.
	s.Remove(e1)
	if s.Len() != 1 {
		t.Errorf("double Remove changed Len() to %d, want 1", s.Len())
	}
	if _, ok := s.Get(e1); ok {
		t.Error("Get on removed entity reported ok")
	}
	if s.Has(Entity(0)) {
		t.Error("zero entity should have no component")
	}
}

func TestStoreSetOnDeadEntityIsNoop(t *testing.T) {
	w := New()
	s := NewStore[int](w)

	dead := w.Create()
	w.Destroy(dead)
	s.Set(dead, 5)
	if s.Has(dead) || s.Len() != 0 {
		t.Error("Set on dead entity was not a no-op")
	}
	if _, ok := s.Get(dead); ok {
		t.Error("Get on dead entity reported ok")
	}

	s.Set(Entity(0), 5)
	if s.Len() != 0 {
		t.Error("Set on zero entity was not a no-op")
	}
}

func TestStoreEntitiesInsertionOrder(t *testing.T) {
	w := New()
	s := NewStore[int](w)
	a, b, c := w.Create(), w.Create(), w.Create()
	s.Set(a, 1)
	s.Set(b, 2)
	s.Set(c, 3)
	if got := s.Entities(); !entitiesEqual(got, []Entity{a, b, c}) {
		t.Fatalf("Entities() = %v, want [a b c]", got)
	}

	// Remove preserves order of the rest; re-insert moves to the end.
	s.Remove(b)
	if got := s.Entities(); !entitiesEqual(got, []Entity{a, c}) {
		t.Fatalf("after Remove: Entities() = %v, want [a c]", got)
	}
	s.Set(b, 20)
	if got := s.Entities(); !entitiesEqual(got, []Entity{a, c, b}) {
		t.Fatalf("after re-insert: Entities() = %v, want [a c b]", got)
	}
}

func TestStoreEntitiesIsSnapshot(t *testing.T) {
	w := New()
	s := NewStore[int](w)
	a := w.Create()
	s.Set(a, 1)
	got := s.Entities()
	got[0] = Entity(999) // mutating the returned slice must not affect the store
	if s.Entities()[0] != a {
		t.Error("Entities() leaked internal storage")
	}
}

func TestStoreEachVisitsInOrderOnce(t *testing.T) {
	w := New()
	s := NewStore[int](w)
	want := make([]Entity, 0, 5)
	for i := 0; i < 5; i++ {
		e := w.Create()
		s.Set(e, i+1)
		want = append(want, e)
	}
	// Interleave a removal so gaps in the order are covered.
	s.Remove(want[2])

	var got []Entity
	seen := map[Entity]int{}
	s.Each(func(e Entity, v *int) {
		got = append(got, e)
		seen[e]++
		*v *= 10
	})
	want = append([]Entity{want[0], want[1]}, want[3:]...)
	if !entitiesEqual(got, want) {
		t.Fatalf("Each visited %v, want %v", got, want)
	}
	for e, n := range seen {
		if n != 1 {
			t.Errorf("entity %v visited %d times, want 1", e, n)
		}
	}
	// In-place mutation through the pointer persists.
	for _, e := range want {
		v, _ := s.Get(e)
		if *v%10 != 0 || *v == 0 {
			t.Errorf("component of %v not mutated: %d", e, *v)
		}
	}
}

func TestStoreEachSurvivesDestroyOfVisited(t *testing.T) {
	w := New()
	s := NewStore[int](w)
	var want []Entity
	for i := 0; i < 4; i++ {
		e := w.Create()
		s.Set(e, i)
		want = append(want, e)
	}

	var got []Entity
	s.Each(func(e Entity, v *int) {
		got = append(got, e)
		w.Destroy(e) // removes the component under the iterator's feet
	})
	if !entitiesEqual(got, want) {
		t.Fatalf("Each visited %v, want %v", got, want)
	}
	if s.Len() != 0 {
		t.Errorf("store Len() = %d after destroying all during Each, want 0", s.Len())
	}
	if w.Len() != 0 {
		t.Errorf("world Len() = %d, want 0", w.Len())
	}
}

func TestStoreEachSurvivesStructuralMutation(t *testing.T) {
	w := New()
	s := NewStore[int](w)
	a, b, c := w.Create(), w.Create(), w.Create()
	s.Set(a, 1)
	s.Set(b, 2)
	s.Set(c, 3)

	var got []Entity
	s.Each(func(e Entity, v *int) {
		got = append(got, e)
		if e == a {
			// Remove another not-yet-visited entity and insert a new one.
			s.Remove(c)
			n := w.Create()
			s.Set(n, 99)
		}
	})
	// c was removed before its turn, so it is skipped; the mid-pass entity
	// is not visited in this pass.
	if !entitiesEqual(got, []Entity{a, b}) {
		t.Fatalf("Each visited %v, want [a b]", got)
	}
	if s.Len() != 3 { // a, b, and the new entity
		t.Errorf("Len() = %d, want 3", s.Len())
	}
}

func TestStoreEachDoesNotVisitEntitiesSetMidIteration(t *testing.T) {
	w := New()
	s := NewStore[int](w)
	a := w.Create()
	s.Set(a, 1)
	b := w.Create()
	s.Set(b, 2)

	var got []Entity
	s.Each(func(e Entity, v *int) {
		got = append(got, e)
		if e == a {
			n := w.Create()
			s.Set(n, 3)  // brand-new entity mid-pass: skipped this pass
			s.Set(b, 20) // replace existing: still visited exactly once
		}
	})
	if !entitiesEqual(got, []Entity{a, b}) {
		t.Fatalf("Each visited %v, want [a b]", got)
	}
	if s.Len() != 3 {
		t.Errorf("Len() = %d, want 3", s.Len())
	}
	if v, _ := s.Get(b); *v != 20 {
		t.Errorf("mid-pass replace lost, got %d", *v)
	}
}
