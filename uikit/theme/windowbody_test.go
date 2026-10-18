package theme

import "testing"

// TestWindowBodyTranslucencyToken pins the ONE window translucency
// token (CONTEXT.md "Window animation" rides the same fill): every
// window body funnels through WindowBody, the value is the ~0.90 alpha
// the feature targets, and the color is the palette's own — windows go
// translucent, they never change hue. The drawn half (every window's
// body sample blending to this exact alpha) is pinned in
// uikit/screens/winchrome_capture_test.go; pixel reads need a running
// ebiten loop, which is the capture gate's job.
func TestWindowBodyTranslucencyToken(t *testing.T) {
	p := DefaultPalette()
	got := WindowBody(p.BG)
	if got.R != p.BG.R || got.G != p.BG.G || got.B != p.BG.B {
		t.Errorf("WindowBody(BG) = %v, want the palette color %v (translucency only)", got, p.BG)
	}
	if got.A != WindowAlpha {
		t.Errorf("WindowBody(BG).A = %d, want the WindowAlpha token %d", got.A, WindowAlpha)
	}
	// ~0.90: within a tenth of a percent of 0.90×255 (229.5 → 230).
	if pct := int(WindowAlpha) * 100 / 255; pct < 90 || pct > 91 {
		t.Errorf("WindowAlpha = %d (%d%%), want ~90%%", WindowAlpha, pct)
	}
	// The title bar keeps the opaque variant — the "slightly more
	// opaque" half of the design stays on the palette's own alpha.
	if p.BGHard.A != 255 {
		t.Errorf("BGHard alpha = %d, want 255 (opaque title bars)", p.BGHard.A)
	}
}
