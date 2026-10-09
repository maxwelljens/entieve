package render

import (
	"math"
	"testing"

	"github.com/entieve/entieve/core"
)

// TestWorldToScreenScreenToWorldRoundTrip verifies the projection pair is a
// true inverse at several zoom levels and window sizes.
func TestWorldToScreenScreenToWorldRoundTrip(t *testing.T) {
	cams := []Camera{
		{Center: core.V(0, 0), Zoom: 1e-3, ScreenW: 1280, ScreenH: 720},
		{Center: core.V(1.234e9, -5.6e8), Zoom: 0.02, ScreenW: 1280, ScreenH: 720},
		{Center: core.V(-42, 7), Zoom: 8e-2, ScreenW: 800, ScreenH: 600},
	}
	pts := []core.Vec2{
		{X: 0, Y: 0},
		{X: 1e9, Y: -1e9},
		{X: -1234.5, Y: 6789.25},
		{X: 1280, Y: 720},
	}
	for ci, cam := range cams {
		for pi, p := range pts {
			x, y := cam.WorldToScreen(p)
			got := cam.ScreenToWorld(x, y)
			if math.Abs(got.X-p.X) > 1e-6*math.Max(1, math.Abs(p.X)) ||
				math.Abs(got.Y-p.Y) > 1e-6*math.Max(1, math.Abs(p.Y)) {
				t.Fatalf("cam %d pt %d: round trip mismatch: in=%v out=%v", ci, pi, p, got)
			}
		}
	}
}

// TestScreenToWorldWorldToScreenIdentity verifies screen -> world -> screen
// is the identity.
func TestScreenToWorldWorldToScreenIdentity(t *testing.T) {
	cam := Camera{Center: core.V(5e8, 2e9), Zoom: 0.017, ScreenW: 1280, ScreenH: 720}
	for _, sp := range [][2]float64{{0, 0}, {640, 360}, {1279, 719}, {12.5, 601.75}} {
		w := cam.ScreenToWorld(sp[0], sp[1])
		x, y := cam.WorldToScreen(w)
		if math.Abs(x-sp[0]) > 1e-4 || math.Abs(y-sp[1]) > 1e-4 {
			t.Fatalf("screen round trip: in=%v got=(%v, %v)", sp, x, y)
		}
	}
}

// TestWorldToScreenZoomScales verifies doubling Zoom doubles the offset from
// screen center.
func TestWorldToScreenZoomScales(t *testing.T) {
	base := Camera{Center: core.V(0, 0), Zoom: 0.01, ScreenW: 1280, ScreenH: 720}
	double := base
	double.Zoom = base.Zoom * 2
	p := core.V(1000, -500)
	x1, y1 := base.WorldToScreen(p)
	x2, y2 := double.WorldToScreen(p)
	dx1, dy1 := x1-base.ScreenW/2, y1-base.ScreenH/2
	dx2, dy2 := x2-double.ScreenW/2, y2-double.ScreenH/2
	if math.Abs(dx2-2*dx1) > 1e-9 || math.Abs(dy2-2*dy1) > 1e-9 {
		t.Fatalf("zoom scaling wrong: dx1=%v dx2=%v dy1=%v dy2=%v", dx1, dx2, dy1, dy2)
	}
}

// TestCameraCenterIsScreenMidpoint: the camera center projects to the middle
// of the screen at any zoom.
func TestCameraCenterIsScreenMidpoint(t *testing.T) {
	for _, zoom := range []float64{1e-3, 0.02, 0.08} {
		cam := Camera{Center: core.V(123, 456), Zoom: zoom, ScreenW: 1280, ScreenH: 720}
		x, y := cam.WorldToScreen(cam.Center)
		if x != cam.ScreenW/2 || y != cam.ScreenH/2 {
			t.Fatalf("zoom %v: center projected to (%v, %v)", zoom, x, y)
		}
	}
}
