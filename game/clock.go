package game

import "github.com/entieve/entieve/sim"

// TimeMultipliers are the selectable time accelerations (cycled with
// [ and ] or - and =).
var TimeMultipliers = [...]int{1, 2, 4, 8}

// maxAccumSeconds caps carried-over scaled time per Advance call so a long
// frame stall cannot trigger a runaway Step spiral.
const maxAccumSeconds = 0.5

// Clock converts real time into fixed-timestep sim Steps at the current
// time-acceleration multiplier. Pure arithmetic — no wall-clock reads.
type Clock struct {
	accum   float64
	multIdx int
}

// Mult returns the current multiplier (1, 2, 4 or 8).
func (c *Clock) Mult() int { return TimeMultipliers[c.multIdx] }

// Bump steps the multiplier up (fwd) or down one notch, clamped to the
// available multipliers. Returns the new multiplier.
func (c *Clock) Bump(fwd bool) int {
	if fwd {
		if c.multIdx < len(TimeMultipliers)-1 {
			c.multIdx++
		}
	} else if c.multIdx > 0 {
		c.multIdx--
	}
	return c.Mult()
}

// Advance accumulates realSec of scaled time and returns the number of
// fixed sim.DT Steps to execute. Leftover sub-tick time is carried to the
// next call; accumulated time is capped at maxAccumSeconds.
func (c *Clock) Advance(realSec float64) int {
	if realSec < 0 {
		realSec = 0
	}
	c.accum += realSec * float64(c.Mult())
	if c.accum > maxAccumSeconds {
		c.accum = maxAccumSeconds
	}
	n := int(c.accum / sim.DT)
	c.accum -= float64(n) * sim.DT
	return n
}
