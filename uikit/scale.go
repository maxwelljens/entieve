package uikit

import "math"

// The global UI scale factor: device DPI × the player's UI-scale setting.
//
// Every length the player sees is authored in DESIGN units — the literal
// numbers in the source, which are exactly today's pixels at scale 1 — and
// multiplied through this funnel at the use site: Px for integer lengths,
// Scale for floats. Font sizes flow the same way: the TypeScale is scaled
// once at environment/kit build (scaleTypeScale), so faces, line heights
// and measure results all follow the scale.
//
// The game owns the value: it calls SetScale(uiScale × deviceScaleFactor)
// at boot and again whenever the settings window changes the scale (the
// interface is rebuilt against the new scale, see game.applyUIScale).
// Sidecar window rects are persisted as design units (px ÷ scale on
// write, Px on read), so one saved layout works at every scale.
//
// Scale 1 is the identity — Px(n) == n and scaleTypeScale is a no-op — so
// the scale-1 layout is bit-identical to the pre-scale pixel layout the
// baseline test suite pins.
var scale = 1.0

// SetScale installs the current scale factor (device DPI × user setting).
// A non-positive, NaN or infinite value falls back to 1: a broken setting
// must never collapse the interface to zero size.
func SetScale(s float64) {
	if !(s > 0) || math.IsNaN(s) || math.IsInf(s, 0) {
		s = 1
	}
	scale = s
}

// Scale returns the current scale factor (1 in tests that never set one).
func Scale() float64 { return scale }

// Px converts one design-unit length to device pixels (rounded). At scale
// 1 it returns n unchanged, which is what keeps every scale-1 test
// byte-identical to the unscaled layout.
func Px(n int) int {
	if scale == 1 {
		return n
	}
	return int(math.Round(float64(n) * scale))
}

// scaleTypeScale multiplies every type-scale step by the current scale —
// the one place font sizes are scaled (call it where a TypeScale is built,
// so every face, line height and measure derived from it agrees).
func scaleTypeScale(ts TypeScale) TypeScale {
	if scale == 1 {
		return ts
	}
	return TypeScale{
		Base:  ts.Base * scale,
		Small: ts.Small * scale,
		Large: ts.Large * scale,
		Tiny:  ts.Tiny * scale,
	}
}
