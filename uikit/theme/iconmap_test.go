package theme

import (
	"image"
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

// TestModuleIconForResolver: a module's own Icon wins when the registry
// knows it; an unknown key or an empty one degrades to the class glyph
// (defence in depth — bad content never renders a broken image), and nil is
// no icon at all.
func TestModuleIconForResolver(t *testing.T) {
	def := &content.ModuleDef{Kind: content.KindWeapon, Icon: "mod_autocannon_s"}
	if got := ModuleIconFor(def); got != "mod_autocannon_s" {
		t.Errorf("module with own icon = %q, want mod_autocannon_s", got)
	}
	if !icons.Has("mod_autocannon_s") {
		t.Error("mod_autocannon_s is not in the manifest")
	}
	def.Icon = "not-an-icon"
	if got, want := ModuleIconFor(def), ModuleIcon(content.KindWeapon); got != want {
		t.Errorf("unknown icon key = %q, want the class glyph %q", got, want)
	}
	def.Icon = ""
	if got, want := ModuleIconFor(def), ModuleIcon(content.KindWeapon); got != want {
		t.Errorf("no icon key = %q, want the class glyph %q", got, want)
	}
	if got := ModuleIconFor(nil); got != "" {
		t.Errorf("ModuleIconFor(nil) = %q, want empty", got)
	}
}

// TestDropinArtWinsEveryIconSlot pins the drop-in precedence: a module's own
// dropped art (keyed by its content id) beats the manifest art entry content
// assigns it, an item's dropped art beats its category silhouette, and a
// corporation's dropped logo beats the emblem — while an unwired def still
// resolves exactly as before.
func TestDropinArtWinsEveryIconSlot(t *testing.T) {
	// mod-autocannon-s ships art both ways: the old manifest entry
	// (mod_autocannon_s) and the player's drop-in. The drop-in is the id.
	def := &content.ModuleDef{ID: "mod-autocannon-s", Kind: content.KindWeapon, Icon: "mod_autocannon_s"}
	if got := ModuleIconFor(def); got != "mod-autocannon-s" {
		t.Errorf("ModuleIconFor = %q, want the drop-in id mod-autocannon-s", got)
	}
	// The five manifest art entries are the ones the drop-in overrides.
	for _, id := range []string{"mod-artillery-s", "mod-artillery-m", "mod-artillery-l", "mod-autocannon-m"} {
		d := &content.ModuleDef{ID: content.ModuleID(id), Kind: content.KindWeapon, Icon: "mod_autocannon_s"}
		if got := ModuleIconFor(d); got != id {
			t.Errorf("%s: ModuleIconFor = %q, want the drop-in id", id, got)
		}
	}
	// Uncovered modules fall back exactly as today: the class glyph.
	if got, want := ModuleIconFor(&content.ModuleDef{ID: "mod-rocket-pod", Kind: content.KindWeapon}), "mod_weapon"; got != want {
		t.Errorf("unwired module = %q, want the class glyph %q", got, want)
	}

	// Items: dropped art first, then the category silhouette.
	for id, cat := range map[content.ItemID]string{
		"min-ferrite": "mineral", "ore-cordite": "ore", "loot-navdata": "loot",
	} {
		if got := ItemIconFor(&content.ItemDef{ID: id, Category: cat}); got != string(id) {
			t.Errorf("ItemIconFor(%s) = %q, want the drop-in id", id, got)
		}
	}
	for cat, want := range map[string]string{
		"ammo": "ammo", "charge": "ammo", "loot": "act_loot",
		"mineral": "kind_belt", "ore": "kind_asteroid", "": icons.Fallback,
	} {
		if got := ItemIconFor(&content.ItemDef{ID: "unwired-item", Category: cat}); got != want {
			t.Errorf("ItemIconFor(category %q) = %q, want %q", cat, got, want)
		}
	}
	if got := ItemIconFor(nil); got != "" {
		t.Errorf("ItemIconFor(nil) = %q, want empty", got)
	}

	// Corporations: the dropped logo, else "" (the caller keeps the emblem).
	for _, corp := range content.AllCorps {
		if got := CorpIconFor(corp.ID); got != corp.ID {
			t.Errorf("CorpIconFor(%s) = %q, want the logo id", corp.ID, got)
		}
	}
	if got := CorpIconFor("corp-nowhere"); got != "" {
		t.Errorf("CorpIconFor(unknown) = %q, want empty (emblem fallback)", got)
	}
}

// TestHeroOutgrowsCorpLogo pins the Show Info hero geometry: the entity's
// icon is strictly bigger than the corporation logo beside it, and the logo
// stays the Sub line's emblem box so the attribution never leads.
func TestHeroOutgrowsCorpLogo(t *testing.T) {
	if IconHero <= IconCorp {
		t.Errorf("hero %d px is not bigger than the corp logo %d px", IconHero, IconCorp)
	}
	if IconCorp != 40 || IconHero != 64 {
		t.Errorf("hero/logo = %d/%d, want 64/40", IconHero, IconCorp)
	}
	if IconHero <= IconTitle || IconCorp <= IconTile {
		t.Error("the hero and logo must outgrow the header/tile boxes they replace")
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
// glyph is the 24 px title box after the dot block, the label starts clear
// of it, and both stay inside the bar.
func TestTitleBarIconAndLabelDoNotOverlap(t *testing.T) {
	env := NewSeam(DefaultPalette(), DefaultTypeScale())
	bar, txt := NewTitleBar(env, "Market", "svc_market")
	layoutTitleBar(t, env, bar)
	if len(bar.Children()) != 3 {
		t.Fatalf("title bar has %d children, want dots + icon + label", len(bar.Children()))
	}
	dots := bar.Children()[0].GetWidget().Rect
	icon := bar.Children()[1].GetWidget().Rect
	label := bar.Children()[2].GetWidget().Rect
	if dots.Dx() != TitleDotsW || dots.Dy() != TitleDot {
		t.Errorf("dot block = %v, want %d×%d", dots, TitleDotsW, TitleDot)
	}
	if dots.Min.X != SpMd {
		t.Errorf("dot block starts at x=%d, want %d (SpMd)", dots.Min.X, SpMd)
	}
	if icon.Dx() != IconTitle || icon.Dy() != IconTitle {
		t.Errorf("icon box = %v, want %d×%d", icon, IconTitle, IconTitle)
	}
	if icon.Min.X != SpMd+TitleDotsW+TitleDotsGap {
		t.Errorf("icon starts at x=%d, want the dot block + gap (%d)",
			icon.Min.X, SpMd+TitleDotsW+TitleDotsGap)
	}
	if dots.Overlaps(icon) || icon.Overlaps(label) {
		t.Errorf("dots %v / icon %v / label %v overlap", dots, icon, label)
	}
	if label.Min.X < icon.Max.X+IconGap {
		t.Errorf("label starts at x=%d, want ≥ %d (icon end + gap)",
			label.Min.X, icon.Max.X+IconGap)
	}
	if !dots.In(bar.GetWidget().Rect) || !icon.In(bar.GetWidget().Rect) || !label.In(bar.GetWidget().Rect) {
		t.Errorf("dots %v / icon %v / label %v escape the bar %v",
			dots, icon, label, bar.GetWidget().Rect)
	}
	if txt.Label != "Market" {
		t.Errorf("title text = %q, want Market", txt.Label)
	}

	// An icon-less bar keeps the text-only layout, still clear of the dots.
	plain, _ := NewTitleBar(env, "Overview", "")
	layoutTitleBar(t, env, plain)
	if len(plain.Children()) != 2 {
		t.Fatalf("icon-less title bar has %d children, want dots + label", len(plain.Children()))
	}
	if plain.Children()[1].GetWidget().Rect.Min.X != SpMd+TitleDotsW+TitleDotsGap {
		t.Errorf("plain title starts at x=%d, want %d (dots + gap)",
			plain.Children()[1].GetWidget().Rect.Min.X, SpMd+TitleDotsW+TitleDotsGap)
	}
}

// TestTitleDots pins the kit's signature mark: three hollow 9 px rings, a
// 1 px border-colour ring each, nothing filled inside.
func TestTitleDots(t *testing.T) {
	p := DefaultPalette()
	img := titleDotsImage(p)
	b := img.Bounds()
	if b.Dx() != TitleDotsW || b.Dy() != TitleDot {
		t.Fatalf("dot image = %v, want %d×%d", b.Size(), TitleDotsW, TitleDot)
	}
	if TitleDot != 9 || TitleDotGap != 7 || TitleDotsW != 41 {
		t.Errorf("dot tokens = %d/%d/%d, want 9/7/41", TitleDot, TitleDotGap, TitleDotsW)
	}
	centre := func(i int) image.Point {
		return image.Pt(i*(TitleDot+TitleDotGap)+TitleDot/2, TitleDot/2)
	}
	for i := 0; i < 3; i++ {
		c := centre(i)
		rim := image.Pt(c.X, c.Y-TitleDot/2) // top of the ring
		if a := img.NRGBAAt(rim.X, rim.Y).A; a < 90 {
			t.Errorf("ring %d rim at %v alpha = %d, want the hairline ink", i, rim, a)
		}
		if a := img.NRGBAAt(c.X, c.Y).A; a > 2 {
			t.Errorf("ring %d centre at %v alpha = %d, want hollow", i, c, a)
		}
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
