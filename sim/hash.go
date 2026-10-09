package sim

import (
	"encoding/json"
	"math"
)

// FNV-1a 64-bit constants.
const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

// hasher is a minimal FNV-1a running hash.
type hasher struct{ h uint64 }

func newHasher() hasher { return hasher{h: fnvOffset} }

func (hh *hasher) byte(b byte) {
	hh.h ^= uint64(b)
	hh.h *= fnvPrime
}

func (hh *hasher) bytes(bs []byte) {
	for _, b := range bs {
		hh.byte(b)
	}
}

func (hh *hasher) str(s string) {
	for i := 0; i < len(s); i++ {
		hh.byte(s[i])
	}
}

func (hh *hasher) u64(v uint64) {
	for i := 0; i < 8; i++ {
		hh.byte(byte(v >> (8 * i)))
	}
}

func (hh *hasher) f64(v float64) { hh.u64(math.Float64bits(v)) }

// hash returns a deterministic fingerprint of the full simulation state.
//
// It is computed as FNV-1a over the canonical Save serialization, which by
// construction contains every piece of persistent state: seed, world and
// system, ticks (Time is an exact function of ticks), wallet, RNG stream
// states (master + every NPC fork), every component of every entity, market
// stock, respawn timers, the hangar, the free-slot mirror and active module
// target bindings. Pending events and queued commands are intentionally
// excluded (Save does not carry them).
//
// Because hash() and Restore() share the same canonical serialization, a
// snapshot -> restore round-trip preserves Hash() exactly, and continued
// execution diverges only if behavior diverges.
func (s *Sim) hash() uint64 {
	save, err := s.buildSave()
	if err != nil {
		// Deterministic fallback (should only trigger on non-finite floats):
		// fingerprint the scalar state plus the error text.
		h := newHasher()
		h.str("fallback:" + err.Error())
		h.u64(uint64(s.seed))
		h.u64(uint64(s.ticks))
		h.f64(s.wallet)
		h.u64(s.RNG.Snapshot())
		return h.h
	}
	raw, err := json.Marshal(save)
	if err != nil {
		h := newHasher()
		h.str("fallback:" + err.Error())
		h.u64(uint64(s.seed))
		h.u64(uint64(s.ticks))
		return h.h
	}
	h := newHasher()
	h.bytes(raw)
	return h.h
}
