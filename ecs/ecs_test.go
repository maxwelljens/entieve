package ecs

import "testing"

func TestEntityHandleParts(t *testing.T) {
	var zero Entity
	if !zero.IsZero() {
		t.Error("zero Entity should be IsZero")
	}
	if zero.Index() != 0 || zero.Generation() != 0 {
		t.Errorf("zero Entity parts = (%d, %d), want (0, 0)", zero.Index(), zero.Generation())
	}

	e := Entity(uint64(3)<<32 | uint64(7))
	if e.IsZero() {
		t.Error("non-zero handle should not be IsZero")
	}
	if e.Index() != 7 {
		t.Errorf("Index() = %d, want 7", e.Index())
	}
	if e.Generation() != 3 {
		t.Errorf("Generation() = %d, want 3", e.Generation())
	}
}

func TestCreateDestroyAliveLen(t *testing.T) {
	w := New()
	if w.Len() != 0 {
		t.Fatalf("fresh world Len() = %d, want 0", w.Len())
	}

	a, b, c := w.Create(), w.Create(), w.Create()
	if a == b || b == c || a == c {
		t.Fatal("Create returned duplicate handles")
	}
	if !w.Alive(a) || !w.Alive(b) || !w.Alive(c) {
		t.Fatal("freshly created entities should be alive")
	}
	if w.Len() != 3 {
		t.Errorf("Len() = %d, want 3", w.Len())
	}

	w.Destroy(b)
	if w.Alive(b) {
		t.Error("destroyed entity still alive")
	}
	if !w.Alive(a) || !w.Alive(c) {
		t.Error("Destroy took down a neighbour")
	}
	if w.Len() != 2 {
		t.Errorf("Len() = %d, want 2", w.Len())
	}

	// Destroying twice is safe.
	w.Destroy(b)
	if w.Len() != 2 {
		t.Errorf("double destroy changed Len() to %d, want 2", w.Len())
	}

	// Destroying an unknown or zero entity is a no-op.
	w.Destroy(Entity(1 << 40))
	w.Destroy(Entity(0))
	if w.Len() != 2 {
		t.Errorf("no-op destroys changed Len() to %d, want 2", w.Len())
	}
	if w.Alive(Entity(0)) {
		t.Error("zero Entity must never be alive")
	}
}

func TestSlotReuseBumpsGeneration(t *testing.T) {
	w := New()
	old := w.Create()
	w.Destroy(old)

	// Before reuse: same generation, but the slot is dead.
	if w.Alive(old) {
		t.Error("destroyed handle alive before slot reuse")
	}

	nw := w.Create()
	if nw.Index() != old.Index() {
		t.Fatalf("expected slot reuse, got index %d (old %d)", nw.Index(), old.Index())
	}
	if nw.Generation() == old.Generation() {
		t.Fatalf("generation not bumped on reuse: old %d, new %d", old.Generation(), nw.Generation())
	}
	if w.Alive(old) {
		t.Error("stale handle alive after slot reuse")
	}
	if !w.Alive(nw) {
		t.Error("reused slot's new entity should be alive")
	}
	if w.Len() != 1 {
		t.Errorf("Len() = %d, want 1", w.Len())
	}
}

func TestWorldDestroyCleansAllStores(t *testing.T) {
	w := New()
	nums := NewStore[int](w)
	labels := NewStore[string](w)

	a, b, c := w.Create(), w.Create(), w.Create()
	nums.Set(a, 1)
	nums.Set(b, 2)
	nums.Set(c, 3)
	labels.Set(a, "a")
	labels.Set(b, "b")
	labels.Set(c, "c")

	w.Destroy(b)
	for name, has := range map[string]bool{"nums": nums.Has(b), "labels": labels.Has(b)} {
		if has {
			t.Errorf("%s still has component of destroyed entity", name)
		}
	}
	if nums.Len() != 2 || labels.Len() != 2 {
		t.Errorf("store lens = (%d, %d), want (2, 2)", nums.Len(), labels.Len())
	}

	// Stale handle cannot re-enter the store.
	nums.Set(b, 99)
	if nums.Has(b) {
		t.Error("Set on destroyed entity was not a no-op")
	}

	w.Destroy(a)
	w.Destroy(c)
	if nums.Len() != 0 || labels.Len() != 0 {
		t.Errorf("stores not empty after destroying all: (%d, %d)", nums.Len(), labels.Len())
	}
	if w.Len() != 0 {
		t.Errorf("world Len() = %d, want 0", w.Len())
	}
}

func TestCreateAfterDestroyNeverReturnsStaleHandle(t *testing.T) {
	w := New()
	s := NewStore[string](w)

	old := w.Create()
	s.Set(old, "old")
	w.Destroy(old)

	if s.Has(old) {
		t.Fatal("component survived world destroy")
	}

	nw := w.Create()
	if w.Alive(old) {
		t.Error("old handle reported alive after reuse")
	}
	// Even if the old handle's slot is now live, its generation is stale,
	// so Set through it must still be a no-op.
	s.Set(old, "zombie")
	if s.Has(old) {
		t.Error("stale handle managed to insert a component")
	}
	s.Set(nw, "new")
	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}
	got := s.Entities()
	if len(got) != 1 || got[0] != nw {
		t.Errorf("Entities() = %v, want [%v]", got, nw)
	}
}
