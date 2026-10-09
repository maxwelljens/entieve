package ui

import (
	"testing"

	"github.com/entieve/entieve/render"
)

// TestFontScaleMatchesRender pins the shared typography scale: ui owns the
// scale (FontBase, one source of truth), render mirrors it for in-world
// labels (render cannot import ui — ui imports render).
func TestFontScaleMatchesRender(t *testing.T) {
	if want := 12 * render.FontScale; FontBase != want {
		t.Fatalf("FontBase = %v, want 12×render.FontScale = %v", FontBase, want)
	}
	if got := 11 * render.FontScale; got != render.LabelSize {
		t.Fatalf("render.LabelSize = %v, want 11×FontScale = %v", render.LabelSize, got)
	}
	if got := 10 * render.FontScale; got != render.LabelSizeSmall {
		t.Fatalf("render.LabelSizeSmall = %v, want 10×FontScale = %v",
			render.LabelSizeSmall, got)
	}
}

// TestTypographyDerivation: every chrome metric derives from font size and
// padding — no hand-tuned pixels survive the ~75% text enlargement.
func TestTypographyDerivation(t *testing.T) {
	if FontSmall != 0.8*FontBase || FontLarge != 1.3*FontBase {
		t.Fatalf("font scale drifted: small=%v base=%v large=%v",
			FontSmall, FontBase, FontLarge)
	}
	if FontBase != 21 {
		t.Fatalf("FontBase = %v, want 21 (≈1.75× the old 12)", FontBase)
	}
	if titleBarH != LineH(FontBase)+3*padSmall {
		t.Fatalf("titleBarH = %v, not derived from FontBase + pad", titleBarH)
	}
	if ovRowH != LineH(FontSmall)+2*padSmall {
		t.Fatalf("ovRowH = %v, want lineH(FontSmall)+2*padSmall = %v",
			ovRowH, LineH(FontSmall)+2*padSmall)
	}
	if ovHeaderH != LineH(FontSmall)+padSmall {
		t.Fatalf("ovHeaderH = %v not derived", ovHeaderH)
	}
	if actBtnH < minHit || minHit != 40 {
		t.Fatalf("action button height %v / minHit %v below the 40 px rule",
			actBtnH, minHit)
	}
	if logH != titleBarH+pad+float64(logLines)*logLineH+pad {
		t.Fatalf("logH = %v not derived from title/pad/line metrics", logH)
	}
	if scannerH != titleBarH+pad+scanStatusH+scanBarH+padSmall+
		float64(scanLines)*scanLineH+pad {
		t.Fatalf("scannerH = %v not derived", scannerH)
	}
	if modH < float64(modRows)*modRowH {
		t.Fatalf("modH %v cannot hold %d rack rows of %v", modH, modRows, modRowH)
	}
	if hudH != 2*pad+3*LineH(FontSmall)+hudCapH+LineH(FontTiny) {
		t.Fatalf("hudH = %v not derived", hudH)
	}
	// Padding is +20% over the legacy 8/4/20 hand-tuned values.
	if pad < 9.6 || pad > 10.4 {
		t.Fatalf("pad = %v, want ≈1.2×8", pad)
	}
	if padSmall < 4.8 || padSmall > 5.2 {
		t.Fatalf("padSmall = %v, want ≈1.2×4", padSmall)
	}
	if padLarge < 23 || padLarge > 25 {
		t.Fatalf("padLarge = %v, want ≈1.2×20", padLarge)
	}
}

// TestTextIsBiggerThanLegacy: the new scale is ~75% larger than the old
// hardcoded sizes (12 → 21, 10/11 → 16.8, 13/14 → 27.3).
func TestTextIsBiggerThanLegacy(t *testing.T) {
	if FontBase < 12*1.7 {
		t.Fatalf("FontBase %v is not ~1.75× the old 12", FontBase)
	}
	if FontSmall < 11*1.4 {
		t.Fatalf("FontSmall %v too small for the old 11px body text", FontSmall)
	}
	if FontLarge < 14*1.7 {
		t.Fatalf("FontLarge %v too small for the old 14px display text", FontLarge)
	}
}
