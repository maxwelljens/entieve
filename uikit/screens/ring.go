package screens

import (
	"image/color"
	"math"

	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// drawCycleRing paints one module cycle ring: a full dim track and the
// remaining fraction from the top, clockwise. The rack's cell, the HUD's
// effect tile and the target card's mini-cell all read the same cycle clock
// only their size, tint and cadence differ. A cancelled cycle still runs
// out, visibly greyed.
func drawCycleRing(dst *ebiten.Image, cx, cy, rad, width float32, remaining float64, lit bool, tint color.RGBA, p theme.Palette) {
	track, fill := iconColor(tint, 60), tint
	if !lit {
		// Cancelled: the cycle still runs out, visibly greyed.
		track, fill = iconColor(p.Dim, 60), p.Dim
	}
	const start = -math.Pi / 2
	render.Arc(dst, cx, cy, rad, start, start+2*math.Pi, width, track)
	if frac := hudFrac01(remaining); frac > 0 {
		render.Arc(dst, cx, cy, rad, start, start+float32(frac*2*math.Pi), width, fill)
	}
}
