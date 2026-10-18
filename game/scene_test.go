package game

import "testing"

// TestZoomRangeSpansSystemToGrid pins the camera's reach at AU scale (the
// rescale documented in docs/SIM.md — "System layout"): the widest framing
// must fit a whole ~80 AU system on the short edge at design size, while the
// tightest framing stays grid-scale — grid UX (km-level zoom, targeting, HUD
// cards) must never move.
func TestZoomRangeSpansSystemToGrid(t *testing.T) {
	const (
		au         = 1.496e11 // house AU conversion
		shortEdge  = 720.0    // design window 1280×720
		longEdge   = 1280.0
		systemSpan = 80.0 // gates at 40 AU radius → 80 AU across
		gridSpan   = 20_000.0
	)
	widest := zoomBase * zoomMin // px/m
	if spanAU := shortEdge / widest / au; spanAU < systemSpan {
		t.Errorf("widest framing spans %.1f AU on the short edge, want >= %.0f AU (whole-system view)",
			spanAU, systemSpan)
	}
	tightest := zoomBase * zoomMax
	if span := longEdge / tightest; span > gridSpan {
		t.Errorf("tightest framing spans %.0f m, want <= %.0f m (grid view)", span, gridSpan)
	}
}
