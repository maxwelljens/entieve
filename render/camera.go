// Package render draws the Entieve simulation: procedural backgrounds,
// celestial bodies, vector ship silhouettes, EVE-style brackets and effects.
//
// Render reads sim state read-only and never mutates it. All drawing is done
// with ebiten/vector primitives and cached procedural sprites.
package render

import "github.com/entieve/entieve/core"

// Camera maps world space (meters) to screen space (pixels). Zoom is pixels
// per meter; a valid projection requires Zoom > 0. All methods are pure.
type Camera struct {
	Center  core.Vec2 // world point at screen center
	Zoom    float64   // pixels per meter
	ScreenW float64
	ScreenH float64
}

// WorldToScreen projects a world position to screen pixels.
func (c Camera) WorldToScreen(p core.Vec2) (x, y float64) {
	return (p.X-c.Center.X)*c.Zoom + c.ScreenW/2,
		(p.Y-c.Center.Y)*c.Zoom + c.ScreenH/2
}

// ScreenToWorld unprojects a screen pixel to a world position.
func (c Camera) ScreenToWorld(x, y float64) core.Vec2 {
	return core.Vec2{
		X: (x-c.ScreenW/2)/c.Zoom + c.Center.X,
		Y: (y-c.ScreenH/2)/c.Zoom + c.Center.Y,
	}
}

// Visible reports whether p projects within the screen, allowing margin
// pixels of slack outside the edges.
func (c Camera) Visible(p core.Vec2, margin float64) bool {
	x, y := c.WorldToScreen(p)
	return x >= -margin && x <= c.ScreenW+margin && y >= -margin && y <= c.ScreenH+margin
}

// clamp bounds v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
