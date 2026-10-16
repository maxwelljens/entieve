package theme

import (
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/hajimehoshi/ebiten/v2"
)

// layoutTitleBar pumps a title bar through a real UI frame so the anchor
// layout has run (rects are zero until a draw).
func layoutTitleBar(t *testing.T, env Theme, bar *widget.Container) {
	t.Helper()
	ui := &ebitenui.UI{Container: bar, PrimaryTheme: env.WidgetTheme()}
	ui.Update()
	ui.Draw(ebiten.NewImage(300, TitleBarH))
}

// TestKindIconCoversEveryKind: every entity kind the read model and the
// content celestial kinds can produce maps to a real icon, and only a
// genuinely unknown kind falls back to the neutral gear.
func TestKindIconCoversEveryKind(t *testing.T) {
	known := []string{
		sim.KindPlayer, sim.KindShip, sim.KindAsteroid, sim.KindWreck,
		sim.KindStation, sim.KindGate, sim.KindBelt, sim.KindStar,
		sim.KindPlanet, sim.KindMoon, sim.KindAnomaly,
		// content celestial kinds share the strings (validCelKinds).
		content.CelStar, content.CelPlanet, content.CelMoon, content.CelStation,
		content.CelBelt, content.CelGate, content.CelAnomaly,
	}
	for _, k := range known {
		name := KindIcon(k)
		if !icons.Has(name) {
			t.Errorf("KindIcon(%q) = %q, which is not in the manifest", k, name)
		}
		if name == icons.Fallback {
			t.Errorf("KindIcon(%q) fell back, but it is a known kind", k)
		}
	}
	// Unknown kinds render the neutral gear, never a missing-image name.
	for _, k := range []string{"", "planetoid", "structure"} {
		if got := KindIcon(k); got != icons.Fallback || !icons.Has(got) {
			t.Errorf("KindIcon(%q) = %q, want the %s fallback", k, got, icons.Fallback)
		}
	}
	// Own ship and hostile share the hull glyph; the row tint tells them
	// apart (ColInfo / ColDanger), so player and ship must agree.
	if KindIcon(sim.KindPlayer) != KindIcon(sim.KindShip) {
		t.Error("player and ship should share the hull icon (tints separate them)")
	}
}

// TestModuleIconCoversEveryModuleKind: each content module kind maps to its
// class icon; an unknown kind still is a module, so it gets the generic
// module gear.
func TestModuleIconCoversEveryModuleKind(t *testing.T) {
	classes := []string{
		content.KindWeapon, content.KindMiningLaser, content.KindStasisWeb,
		content.KindWarpScram, content.KindShieldBoost, content.KindShieldExtender,
		content.KindArmorRep, content.KindArmorPlate, content.KindResistCoat,
		content.KindCapBooster, content.KindCapRecharger, content.KindAfterburner,
	}
	seen := map[string]string{}
	for _, k := range classes {
		name := ModuleIcon(k)
		if !icons.Has(name) {
			t.Errorf("ModuleIcon(%q) = %q, which is not in the manifest", k, name)
		}
		seen[name] = k
	}
	// Six class glyphs ship: weapon / mining / web / scram / defense / utility.
	if len(seen) != 6 {
		t.Errorf("%d distinct module glyphs for %d kinds, want 6 (map above is %v)",
			len(seen), len(classes), seen)
	}
	if got := ModuleIcon("hyperspace-lance"); got != "row_module" {
		t.Errorf("unknown module kind = %q, want the generic module gear", got)
	}
	if !icons.Has("row_module") {
		t.Error("row_module missing from the manifest")
	}
}

// TestContractIconCoversContractKinds: kill/deliver/mine get their board
// glyphs and anything else falls back.
func TestContractIconCoversContractKinds(t *testing.T) {
	want := map[string]string{
		content.ContractKill:    "ctr_kill",
		content.ContractDeliver: "ctr_deliver",
		content.ContractMine:    "ctr_mine",
	}
	for kind, name := range want {
		if got := ContractIcon(kind); got != name {
			t.Errorf("ContractIcon(%q) = %q, want %q", kind, got, name)
		}
		if !icons.Has(name) {
			t.Errorf("%q is not in the manifest", name)
		}
	}
	if got := ContractIcon("escort"); got != icons.Fallback {
		t.Errorf("ContractIcon(escort) = %q, want the fallback", got)
	}
}

// TestTitleBarIconAndLabelDoNotOverlap pins the title bar's geometry: the
// glyph is the 24 px title box at the bar's start, the label starts clear
// of it, and both stay inside the bar.
func TestTitleBarIconAndLabelDoNotOverlap(t *testing.T) {
	env := NewSeam(DefaultPalette(), DefaultTypeScale())
	bar, txt := NewTitleBar(env, "Market", "svc_market")
	layoutTitleBar(t, env, bar)
	if len(bar.Children()) != 2 {
		t.Fatalf("title bar has %d children, want icon + label", len(bar.Children()))
	}
	icon := bar.Children()[0].GetWidget().Rect
	label := bar.Children()[1].GetWidget().Rect
	if icon.Dx() != IconTitle || icon.Dy() != IconTitle {
		t.Errorf("icon box = %v, want %d×%d", icon, IconTitle, IconTitle)
	}
	if icon.Overlaps(label) {
		t.Errorf("icon %v overlaps the label %v", icon, label)
	}
	if icon.Min.X != ContentsPad {
		t.Errorf("icon starts at x=%d, want the %d px bar padding", icon.Min.X, ContentsPad)
	}
	if label.Min.X < icon.Max.X+IconGap {
		t.Errorf("label starts at x=%d, want ≥ %d (icon end + gap)",
			label.Min.X, icon.Max.X+IconGap)
	}
	if !icon.In(bar.GetWidget().Rect) || !label.In(bar.GetWidget().Rect) {
		t.Errorf("icon %v / label %v escape the bar %v",
			icon, label, bar.GetWidget().Rect)
	}
	if txt.Label != "Market" {
		t.Errorf("title text = %q, want Market", txt.Label)
	}

	// An icon-less bar keeps the old text-only layout.
	plain, _ := NewTitleBar(env, "Overview", "")
	layoutTitleBar(t, env, plain)
	if len(plain.Children()) != 1 {
		t.Fatalf("icon-less title bar has %d children, want 1", len(plain.Children()))
	}
	if plain.Children()[0].GetWidget().Rect.Min.X != ContentsPad {
		t.Errorf("plain title starts at x=%d, want %d",
			plain.Children()[0].GetWidget().Rect.Min.X, ContentsPad)
	}
}

// TestIconSizesAreTheTypeGrid pins the three sizes every surface draws at.
func TestIconSizesAreTheTypeGrid(t *testing.T) {
	if IconInline != 16 || IconButton != 20 || IconTitle != 24 {
		t.Errorf("icon grid = %d/%d/%d, want 16/20/24", IconInline, IconButton, IconTitle)
	}
	if IconGap != 4 || IconDisabledA != 102 {
		t.Errorf("icon gap = %d, disabled alpha = %d, want 4/102", IconGap, IconDisabledA)
	}
}
