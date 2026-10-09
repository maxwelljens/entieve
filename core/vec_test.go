package core

import (
	"math"
	"testing"
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < eps }

func nearVec(a, b Vec2) bool { return near(a.X, b.X) && near(a.Y, b.Y) }

func TestVec2VectorOps(t *testing.T) {
	tests := []struct {
		name string
		got  Vec2
		want Vec2
	}{
		{"add", V(1, 2).Add(V(3, -4)), V(4, -2)},
		{"sub", V(1, 2).Sub(V(3, -4)), V(-2, 6)},
		{"scale", V(3, -4).Scale(2.5), V(7.5, -10)},
		{"neg", V(3, -4).Neg(), V(-3, 4)},
		{"neg zero", V(0, 0).Neg(), V(0, 0)},
		{"perp", V(1, 2).Perp(), V(-2, 1)},
		{"perp twice is negation", V(1, 2).Perp().Perp(), V(-1, -2)},
		{"norm", V(3, 4).Norm(), V(0.6, 0.8)},
		{"norm zero", V(0, 0).Norm(), V(0, 0)},
		{"clamp below max", V(3, 4).ClampLen(10), V(3, 4)},
		{"clamp at max", V(3, 4).ClampLen(5), V(3, 4)},
		{"clamp above max", V(3, 4).ClampLen(2.5), V(1.5, 2)},
		{"clamp zero vector", V(0, 0).ClampLen(1), V(0, 0)},
		{"rotate 0", V(1, 0).Rotate(0), V(1, 0)},
		{"rotate pi/2", V(1, 0).Rotate(math.Pi / 2), V(0, 1)},
		{"rotate pi", V(1, 0).Rotate(math.Pi), V(-1, 0)},
		{"rotate -pi/2", V(1, 0).Rotate(-math.Pi / 2), V(0, -1)},
		{"lerp t=0", V(0, 0).Lerp(V(10, 20), 0), V(0, 0)},
		{"lerp t=1", V(0, 0).Lerp(V(10, 20), 1), V(10, 20)},
		{"lerp t=0.5", V(0, 0).Lerp(V(10, 20), 0.5), V(5, 10)},
		{"lerp from non-origin", V(2, 2).Lerp(V(4, 6), 0.5), V(3, 4)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !nearVec(tc.got, tc.want) {
				t.Errorf("got (%v, %v), want (%v, %v)", tc.got.X, tc.got.Y, tc.want.X, tc.want.Y)
			}
		})
	}
}

func TestVec2RotatePreservesLength(t *testing.T) {
	v := V(3, 4)
	for _, theta := range []float64{0, 0.7, math.Pi / 3, math.Pi, -1.234} {
		if got := v.Rotate(theta).Len(); !near(got, v.Len()) {
			t.Errorf("Rotate(%v).Len() = %v, want %v", theta, got, v.Len())
		}
	}
}

func TestVec2ScalarOps(t *testing.T) {
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"dot orthogonal", V(1, 0).Dot(V(0, 1)), 0},
		{"dot aligned", V(1, 2).Dot(V(3, 4)), 11},
		{"dot opposed", V(1, 0).Dot(V(-2, 0)), -2},
		{"len 3-4-5", V(3, 4).Len(), 5},
		{"len zero", V(0, 0).Len(), 0},
		{"len2", V(3, 4).Len2(), 25},
		{"dist", V(1, 1).Dist(V(4, 5)), 5},
		{"dist zero", V(2, 3).Dist(V(2, 3)), 0},
		{"dist2", V(1, 1).Dist2(V(4, 5)), 25},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !near(tc.got, tc.want) {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

// TestVec2Angle pins the documented convention: radians, 0 = +X, and because
// screen Y grows downward the positive direction is clockwise on screen.
func TestVec2Angle(t *testing.T) {
	tests := []struct {
		name string
		v    Vec2
		want float64
	}{
		{"+X is 0", V(1, 0), 0},
		{"+Y is +pi/2", V(0, 1), math.Pi / 2},
		{"-X is pi", V(-1, 0), math.Pi},
		{"-Y is -pi/2", V(0, -1), -math.Pi / 2},
		{"diagonal", V(1, 1), math.Pi / 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.Angle(); !near(got, tc.want) {
				t.Errorf("Angle() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVec2IsZero(t *testing.T) {
	tests := []struct {
		v    Vec2
		want bool
	}{
		{V(0, 0), true},
		{V(0, 1), false},
		{V(1, 0), false},
		{V(-0.5, 0), false},
	}
	for _, tc := range tests {
		if got := tc.v.IsZero(); got != tc.want {
			t.Errorf("V(%v, %v).IsZero() = %v, want %v", tc.v.X, tc.v.Y, got, tc.want)
		}
	}
}
