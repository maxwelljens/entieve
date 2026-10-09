// Package core provides deterministic math helpers shared by simulation and
// rendering. Everything in this package is pure: no globals, no platform
// dependencies, and identical inputs always produce identical outputs so that
// replays and tests stay stable.
package core

import "math"

// Vec2 is a 2D vector of float64 components.
type Vec2 struct{ X, Y float64 }

// V constructs a Vec2.
func V(x, y float64) Vec2 { return Vec2{X: x, Y: y} }

// Add returns v + o.
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{X: v.X + o.X, Y: v.Y + o.Y} }

// Sub returns v - o.
func (v Vec2) Sub(o Vec2) Vec2 { return Vec2{X: v.X - o.X, Y: v.Y - o.Y} }

// Scale returns v scaled by k.
func (v Vec2) Scale(k float64) Vec2 { return Vec2{X: v.X * k, Y: v.Y * k} }

// Neg returns v with both components negated.
func (v Vec2) Neg() Vec2 { return Vec2{X: -v.X, Y: -v.Y} }

// Dot returns the dot product of v and o.
func (v Vec2) Dot(o Vec2) float64 { return v.X*o.X + v.Y*o.Y }

// Perp returns the perpendicular of v, rotated a quarter turn: (-Y, X).
func (v Vec2) Perp() Vec2 { return Vec2{X: -v.Y, Y: v.X} }

// Len returns the Euclidean length of v.
func (v Vec2) Len() float64 { return math.Hypot(v.X, v.Y) }

// Len2 returns the squared length of v, avoiding the square root.
func (v Vec2) Len2() float64 { return v.X*v.X + v.Y*v.Y }

// Dist returns the distance between v and o.
func (v Vec2) Dist(o Vec2) float64 { return math.Hypot(v.X-o.X, v.Y-o.Y) }

// Dist2 returns the squared distance between v and o, avoiding the square root.
func (v Vec2) Dist2(o Vec2) float64 { d := v.Sub(o); return d.Len2() }

// Norm returns v normalized to unit length. The zero vector returns zero.
func (v Vec2) Norm() Vec2 {
	l := v.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{X: v.X / l, Y: v.Y / l}
}

// ClampLen limits v to at most max in length, preserving direction. Vectors
// already shorter than max are returned unchanged. A non-positive max
// returns the zero vector for non-zero input.
func (v Vec2) ClampLen(max float64) Vec2 {
	if max <= 0 {
		return Vec2{}
	}
	l := v.Len()
	if l <= max {
		return v
	}
	return v.Scale(max / l)
}

// Angle returns the direction of v in radians: 0 = +X, growing toward +Y.
// In screen space Y grows downward, so positive angles turn clockwise on
// screen.
func (v Vec2) Angle() float64 { return math.Atan2(v.Y, v.X) }

// Rotate returns v rotated by theta radians counter-clockwise in math
// coordinates (clockwise on screen, see Angle).
func (v Vec2) Rotate(theta float64) Vec2 {
	c, s := math.Cos(theta), math.Sin(theta)
	return Vec2{X: v.X*c - v.Y*s, Y: v.X*s + v.Y*c}
}

// Lerp linearly interpolates from v to o by t. t is not clamped.
func (v Vec2) Lerp(o Vec2, t float64) Vec2 {
	return Vec2{X: v.X + (o.X-v.X)*t, Y: v.Y + (o.Y-v.Y)*t}
}

// IsZero reports whether v is exactly the zero vector.
func (v Vec2) IsZero() bool { return v.X == 0 && v.Y == 0 }
