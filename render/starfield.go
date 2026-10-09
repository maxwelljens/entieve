package render

import (
	"hash/fnv"

	"github.com/entieve/entieve/core"
)

// Star is one starfield point in unit-cell coordinates (both in [0,1)).
type Star struct {
	X, Y   float64
	Size   float64 // relative radius
	Bright float64 // alpha multiplier in (0,1]
}

// starfieldSalt separates the starfield stream from other RNG uses.
const starfieldSalt = 0x51A7F00D

// Starfield returns count star points derived purely from seed: identical
// seeds always produce identical starfields (determinism contract; tested).
func Starfield(seed uint64, count int) []Star {
	if count < 0 {
		count = 0
	}
	rng := core.RestoreRNG(seed ^ starfieldSalt)
	stars := make([]Star, count)
	for i := range stars {
		stars[i] = Star{
			X:      rng.Float64(),
			Y:      rng.Float64(),
			Size:   1 + rng.Float64()*1.6,
			Bright: 0.35 + 0.65*rng.Float64(),
		}
	}
	return stars
}

// HashID hashes a string ID (e.g. a system ID) into a stable seed.
func HashID(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
