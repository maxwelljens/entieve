package sim

import (
	"encoding/json"
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

// hash returns a deterministic fingerprint of the full simulation state.
//
// It is computed as FNV-1a over the canonical Save serialization, which by
// construction contains every piece of persistent state: seed, world and
// system, ticks (Time is an exact function of ticks), wallet, RNG stream
// states (master + every NPC fork), every component of every entity, the
// per-station market books (orders with station/side/owner/arrival id, the
// last-trade rungs and the next order id), respawn timers, the hangar, the
// free-slot mirror, the respawn anchor, active module target bindings, the
// manufacturing jobs and the per-station craft counters.
// Pending events and queued commands are intentionally excluded (Save does
// not carry them).
//
// Because hash() and Restore() share the same canonical serialization, a
// snapshot -> restore round-trip preserves Hash() exactly, and continued
// execution diverges only if behavior diverges.
//
// There is no degraded fallback: if the state cannot be serialized (a
// non-finite float somewhere in the Save), hash() panics through hashFail
// rather than returning a plausible-looking value built from partial state.
func (s *Sim) hash() uint64 {
	save, err := s.buildSave()
	if err != nil {
		hashFail("save", err)
	}
	raw, err := json.Marshal(save)
	if err != nil {
		hashFail("json", err)
	}
	h := newHasher()
	h.bytes(raw)
	return h.h
}

// hashFail reports an unusable fingerprint. There is deliberately no
// degraded fallback: a hash built from partial state (seed/ticks/wallet)
// looks like a real fingerprint, so two broken twin runs would "agree" on
// two meaningless values and pass silently. A non-finite float in the Save
// is a determinism bug — fail loudly instead (F8).
func hashFail(stage string, err error) {
	panic("sim: Hash() cannot fingerprint the state (" + stage + "): " + err.Error())
}
