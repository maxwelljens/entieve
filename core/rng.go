package core

import "math"

// RNG is a deterministic, platform-independent pseudo-random number
// generator.
//
// The exact algorithm (SplitMix64) is part of the stable API: simulation
// determinism, replay tests, and golden-value tests depend on every stream
// staying bit-identical across platforms and releases. Do not change the
// algorithm, the seeding, or the bit extraction rules without treating it as
// a breaking change.
//
// RNG is not safe for concurrent use.
type RNG struct{ state uint64 }

// splitMix64 is the reference mixing step. Pure uint64 arithmetic, so results
// are identical on every platform.
func splitMix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	z := x
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// NewRNG returns an RNG seeded from seed.
func NewRNG(seed int64) *RNG { return &RNG{state: uint64(seed)} }

// Uint64 returns the next pseudo-random uint64 in the stream.
func (r *RNG) Uint64() uint64 {
	r.state = splitMix64(r.state)
	return r.state
}

// Intn returns a pseudo-random int in [0, n). It returns 0 when n <= 0.
// The result is unbiased (modulo rejection sampling).
func (r *RNG) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	// Rejection sampling keeps the result uniform: reject the tail of the
	// 2^64 range that is not a multiple of n.
	bound := uint64(n)
	threshold := -bound % bound
	for {
		v := r.Uint64()
		if v >= threshold {
			return int(v % bound)
		}
	}
}

// Float64 returns a pseudo-random float in [0, 1) using the top 53 bits
// divided by 2^53.
func (r *RNG) Float64() float64 {
	return float64(r.Uint64()>>11) / (1 << 53)
}

// NormFloat64 returns a standard normally distributed sample (mean 0,
// variance 1) drawn via the Box-Muller transform. The second value of each
// pair is discarded.
func (r *RNG) NormFloat64() float64 {
	for {
		// Shift away the sign/exponent so u1 is in (0, 1).
		u1 := float64(r.Uint64()>>11) / (1 << 53)
		if u1 > 0 {
			u2 := r.Float64()
			return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
		}
	}
}

// Range returns a pseudo-random float in [min, max).
func (r *RNG) Range(min, max float64) float64 { return min + r.Float64()*(max-min) }

// Chance reports true with probability p. p <= 0 never fires, p >= 1 always
// fires.
func (r *RNG) Chance(p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}
	return r.Float64() < p
}

// Fork returns a new RNG seeded with this RNG's current state XOR
// splitMix64(stream). The derivation is deterministic, so Fork with the same
// receiver state and stream id always yields an identical stream, and
// different stream ids yield independent streams. This is a stable API: use
// it to give independent subsystems independent, reproducible streams.
func (r *RNG) Fork(stream uint64) *RNG {
	return &RNG{state: r.state ^ splitMix64(stream)}
}

// Snapshot returns the current internal state, suitable for RestoreRNG.
func (r *RNG) Snapshot() uint64 { return r.state }

// RestoreRNG returns an RNG whose stream continues from a state captured by
// Snapshot.
func RestoreRNG(state uint64) *RNG { return &RNG{state: state} }
