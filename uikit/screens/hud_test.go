package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/hulls"
	"github.com/entieve/entieve/uikit/theme"
)

// TestHudConstruction checks the panel builds and Sync fills it from the
// showcase sim without panicking.
func TestHudConstruction(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	if h.Container() == nil {
		t.Fatal("Hud.Container() is nil")
	}
	if h.hp[0].label != "" {
		t.Errorf("shield value before Sync = %q, want empty", h.hp[0].label)
	}
	h.Sync()
	if h.hp[0].label == "" {
		t.Error("Sync did not populate the shield value")
	}
}

// TestHudSyncReadouts pins every HUD readout to the showcase sim state.
func TestHudSyncReadouts(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.Sync()

	// showcase start: sloop 500/500 shield, 500/500 armor, 600/600
	// structure, cap 1800/1800 GJ, wallet 10,000,000.00 €, speed 0.
	for _, c := range []struct{ name, got, want string }{
		{"shield", h.hp[0].label, "500 / 500"},
		{"armor", h.hp[1].label, "500 / 500"},
		{"structure", h.hp[2].label, "600 / 600"},
		{"speed", h.speedLbl.Label, theme.FormatSpeed(0)},
		{"time", h.timeLbl.Label, "×1 time"},
		{"wallet", h.walletLbl.Label, "10,000,000.00 €"},
		{"capacitor", h.capValue.Label, "1,800.0/1,800.0 GJ"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if vis := h.recharging.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("full capacitor shows the recharging caption (visibility %d)", vis)
	}
	if h.capBar.GetCurrent() != hudCapMax {
		t.Errorf("cap bar = %d, want full (%d)", h.capBar.GetCurrent(), hudCapMax)
	}

	// Draining the capacitor shows the caption and moves the bar.
	cap, ok := env.Sim.Caps.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no capacitor")
	}
	cap.Current = 450 // 25 %
	h.Sync()
	if vis := h.recharging.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Errorf("drained capacitor hides the recharging caption (visibility %d)", vis)
	}
	if want := int(0.25 * hudCapMax); h.capBar.GetCurrent() != want {
		t.Errorf("cap bar = %d, want %d", h.capBar.GetCurrent(), want)
	}
	if h.capValue.Label != "450.0/1,800.0 GJ" {
		t.Errorf("cap label = %q", h.capValue.Label)
	}

	// Time acceleration follows SetTimeScale.
	h.SetTimeScale(4)
	h.Sync()
	if h.timeLbl.Label != "×4 time" {
		t.Errorf("time label = %q, want %q", h.timeLbl.Label, "×4 time")
	}

	// Wallet formatting (incl. the negative, sub-euro and carry cases) —
	// the exported formatter every money readout shares (HUD, market title,
	// the station board's payout line).
	for _, c := range []struct {
		in   float64
		want string
	}{
		{40000, "40,000.00 €"},
		{12345.67, "12,345.67 €"},
		{1, "1.00 €"},
		{-1234.5, "-1,234.50 €"},
		{0.999, "1.00 €"},
		{0, "0.00 €"},
		{0.5, "50¢"},
		{0.99, "99¢"},
		{-0.5, "-50¢"},
	} {
		if got := theme.FormatCredits(c.in); got != c.want {
			t.Errorf("FormatCredits(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHudStatusOnDamage pins the health bars, the inside values and the
// hull portrait: bars deplete with the layer fractions, the portrait
// repaints only when the hull changes and is the fitted hull's raster.
func TestHudStatusOnDamage(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.Sync()
	sh, ok := env.Sim.Ships.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no ship")
	}
	if h.hp[0].frac != 1 || h.hp[1].frac != 1 || h.hp[2].frac != 1 {
		t.Fatalf("full health fractions = %v, want all full", h.hp)
	}
	if h.hpDraws != 1 {
		t.Fatalf("first Sync repainted the health bars %d times, want 1", h.hpDraws)
	}
	if h.portraitDraws != 1 || h.portraitHull != sh.Hull {
		t.Fatalf("portrait draws=%d hull=%q, want one paint of %q",
			h.portraitDraws, h.portraitHull, sh.Hull)
	}
	if !hulls.Has(sh.Hull) {
		t.Errorf("hulls.Has(%q) = false for the fitted hull", sh.Hull)
	}
	if b := h.portrait.Bounds(); b.Dx() != hudPortrait || b.Dy() != hudPortrait {
		t.Fatalf("portrait image %v, want %dx%d", b, hudPortrait, hudPortrait)
	}

	h.Sync() // nothing changed → no portrait repaint
	if h.portraitDraws != 1 {
		t.Errorf("idle Sync repainted the portrait (%d)", h.portraitDraws)
	}

	sh.Shield = 125 // 25 %
	h.Sync()
	if h.hp[0].frac != 0.25 || h.hpDraws != 2 {
		t.Errorf("shield bar = %v (draws %d), want 0.25 / 2", h.hp[0].frac, h.hpDraws)
	}
	if h.hp[0].label != "125 / 500" {
		t.Errorf("shield value = %q", h.hp[0].label)
	}
	if h.hp[1].label != "500 / 500" || h.hp[2].label != "600 / 600" {
		t.Errorf("armor/structure values = %q/%q, want untouched", h.hp[1].label, h.hp[2].label)
	}
	h.Sync()
	if h.hpDraws != 2 {
		t.Errorf("idle Sync repainted the health bars (%d)", h.hpDraws)
	}

	// A hull change repaints the portrait (the art follows the hull).
	sh.Hull = "ship-other"
	h.Sync()
	if h.portraitDraws != 2 || h.portraitHull != "ship-other" {
		t.Errorf("hull change draws=%d hull=%q, want 2 / ship-other",
			h.portraitDraws, h.portraitHull)
	}
}

// TestWedgeGlyphFits pins the speed glyph inside the wedge image at the
// doubled stat size: the bottom-left placement is clamped, never clipped.
func TestWedgeGlyphFits(t *testing.T) {
	for size := 4; size <= theme.StatIconSize; size++ {
		x, y := wedgeGlyphXY(hudWedgeW, hudWedgeH, size)
		if x < 0 || y < 0 || x+size > hudWedgeW || y+size > hudWedgeH {
			t.Errorf("size %d: glyph %dx%d at %d,%d escapes the %dx%d wedge",
				size, size, size, x, y, hudWedgeW, hudWedgeH)
		}
	}
}

// TestHudVelocityWedge pins the wedge fraction: current / base max speed
// (a stable top), clamped, zero when max is unknown.
func TestHudVelocityWedge(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.Sync()
	if h.wedgeFrac != 0 || h.wedgeDraws != 1 {
		t.Fatalf("at-rest wedge = %v (draws %d), want 0 / one paint", h.wedgeFrac, h.wedgeDraws)
	}
	if b := h.wedge.Bounds(); b.Dx() != hudWedgeW || b.Dy() != hudWedgeH {
		t.Errorf("wedge image %v, want %dx%d (the health band's height)", b, hudWedgeW, hudWedgeH)
	}
	m, ok := env.Sim.Motions.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no motion")
	}
	m.MaxSpeed = 400
	m.Vel = core.V(100, 0)
	h.Sync()
	if h.wedgeFrac != 0.25 || h.wedgeDraws != 2 {
		t.Errorf("wedge = %v (draws %d), want 0.25 / 2", h.wedgeFrac, h.wedgeDraws)
	}
	if h.speedLbl.Label != theme.FormatSpeed(100) {
		t.Errorf("speed = %q, want %q", h.speedLbl.Label, theme.FormatSpeed(100))
	}
	h.Sync()
	if h.wedgeDraws != 2 {
		t.Errorf("idle Sync repainted the wedge (%d)", h.wedgeDraws)
	}
	m.Vel = core.V(10_000, 0) // clamped to full
	h.Sync()
	if h.wedgeFrac != 1 {
		t.Errorf("over-speed wedge = %v, want 1", h.wedgeFrac)
	}
	m.MaxSpeed = 0
	h.Sync()
	if h.wedgeFrac != 0 {
		t.Errorf("unknown max speed wedge = %v, want 0", h.wedgeFrac)
	}
}

// TestHudTargetBoxes covers the top-center lock row: hidden with no locks,
// one card per lock with name, distance, lock progress and layer fractions,
// hidden again when the locks drop. The card state is the render input, so
// pinning it pins what gets drawn.
func TestHudTargetBoxes(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	s := env.Sim
	player := s.Player()
	h.Sync()
	if vis := h.targetRow.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Fatalf("no locks but target row visibility = %d", vis)
	}

	// Find a ship and a non-ship on the grid (the showcase starts with
	// off-grid rats and a belt of rocks).
	var ship, rock sim.Info
	var haveShip, haveRock bool
	for _, inf := range s.Grid() {
		if inf.Entity == player {
			continue
		}
		if inf.Kind == sim.KindShip && !haveShip {
			ship, haveShip = inf, true
		} else if inf.Kind == sim.KindAsteroid && !haveRock {
			rock, haveRock = inf, true
		}
	}
	if !haveShip || !haveRock {
		t.Fatalf("showcase grid missing a ship/asteroid pair (ship=%v rock=%v)", haveShip, haveRock)
	}

	tg, ok := s.Targetings.Get(player)
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{
		{Target: ship.Entity, Progress: 0.42},
		{Target: rock.Entity, Progress: 1},
	}
	h.Sync()
	if vis := h.targetRow.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Fatalf("locked targets hide the target row (visibility %d)", vis)
	}
	b0, b1, b2 := h.boxes[0], h.boxes[1], h.boxes[2]

	// Card 0: the ship target — name, distance, lock progress and the
	// three real layer fractions.
	wantName, wantDist := h.boxLabels(ship.Name, theme.FormatDistance(ship.Distance))
	if b0.card.name != wantName {
		t.Errorf("box 0 name = %q, want %q", b0.card.name, wantName)
	}
	if b0.card.dist != wantDist || wantDist == "" {
		t.Errorf("box 0 distance = %q, want the formatted %v", b0.card.dist, ship.Distance)
	}
	if b0.card.lock != 0.42 {
		t.Errorf("box 0 lock = %v, want 0.42", b0.card.lock)
	}
	if !b0.card.ships {
		t.Error("box 0 target is a ship but the card reports a single layer")
	}
	sh, ok := s.Ships.Get(ship.Entity)
	if !ok {
		t.Fatal("locked ship target has no ShipState")
	}
	want := [3]float64{
		hudFrac(sh.Shield, sh.MaxShield),
		hudFrac(sh.Armor, sh.MaxArmor),
		hudFrac(sh.Structure, sh.MaxStructure),
	}
	if b0.card.layers != want {
		t.Errorf("box 0 layers = %v, want %v", b0.card.layers, want)
	}

	// Card 1: the asteroid — full lock, a single health fraction.
	if b1.card.lock != 1 {
		t.Errorf("box 1 lock = %v, want 1", b1.card.lock)
	}
	if b1.card.ships {
		t.Error("box 1 target is an asteroid but the card reports three layers")
	}
	if b1.card.layers[0] != rock.Health {
		t.Errorf("box 1 health = %v, want %v", b1.card.layers[0], rock.Health)
	}
	if b1.card.dist == "" {
		t.Error("box 1 distance empty")
	}
	if vis := b2.root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("third box visible with two locks (visibility %d)", vis)
	}

	// Repaint discipline: a state change repaints, an idle Sync does not.
	if b0.draws == 0 {
		t.Fatal("box 0 never painted its card image")
	}
	draws := b0.draws
	h.Sync()
	if b0.draws != draws {
		t.Errorf("idle Sync repainted box 0 (%d → %d)", draws, b0.draws)
	}
	sh.Shield = sh.MaxShield * 0.5
	h.Sync()
	if b0.draws != draws+1 {
		t.Errorf("damage did not repaint box 0 (draws %d, want %d)", b0.draws, draws+1)
	}
	if b0.drawn.layers[0] != 0.5 {
		t.Errorf("repainted layer = %v, want 0.5", b0.drawn.layers[0])
	}

	tg.Locks = nil
	h.Sync()
	if vis := h.targetRow.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("unlocked targets keep the row visible (visibility %d)", vis)
	}
}

// TestHudDrawSmoke draws the HUD + rack at both design sizes inside an
// interface root and checks the anchor contract: one bottom-center pair
// (status left, rack right, one baseline), the strip above the status, and
// the target row top-center. At 1280 the pair clears the log's left pocket.
func TestHudDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.MountRack(NewRack(env).Container())
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.Frame(t, 1280, 720)

	// One lock so the target row is laid out, then re-anchor both halves.
	var target sim.Info
	for _, inf := range env.Sim.Grid() {
		if inf.Entity != env.Sim.Player() {
			target = inf
			break
		}
	}
	tg, ok := env.Sim.Targetings.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{{Target: target.Entity, Progress: 1}}
	c.Frame(t, 1280, 720)

	pair := h.pair.GetWidget().Rect
	status := h.status.GetWidget().Rect
	strip := h.strip.GetWidget().Rect
	rack := h.pair.Children()[1].GetWidget().Rect
	if pair.Max.Y < 709 || pair.Max.Y > 711 {
		t.Errorf("pair bottom = %d, want 710 (bottom-centred)", pair.Max.Y)
	}
	if want := (1280 - pair.Dx()) / 2; pair.Min.X < want-1 || pair.Min.X > want+1 {
		t.Errorf("pair x = %d, want %d (centred)", pair.Min.X, want)
	}
	if pair.Min.X <= 361 {
		t.Errorf("pair left = %d, want clear of the log pocket (right edge 361)", pair.Min.X)
	}
	if status.Max.Y != pair.Max.Y {
		t.Errorf("status bottom = %d, want the pair baseline %d", status.Max.Y, pair.Max.Y)
	}
	if rack.Max.Y != pair.Max.Y || rack.Min.X != status.Max.X+theme.Px(hudPairGap) {
		t.Errorf("rack %v, want bottom-aligned right of the status %v", rack, status)
	}
	if strip.Max.Y > status.Min.Y || strip.Dx() != status.Dx() || strip.Min.X != status.Min.X {
		t.Errorf("strip %v, want same width immediately above the status %v", strip, status)
	}

	// The velocity wedge shares the health bars' band (bars left, wedge
	// right) and the capacitor bar spans the whole column beneath them.
	hp := h.healthGfx.GetWidget().Rect
	wedge := h.wedgeGfx.GetWidget().Rect
	capr := h.capBar.GetWidget().Rect
	if wedge.Min.Y != hp.Min.Y || wedge.Max.Y != hp.Max.Y {
		t.Errorf("wedge %v, want the health bars' band %v", wedge, hp)
	}
	if wedge.Min.X != hp.Max.X+theme.Px(hudColGap) {
		t.Errorf("wedge %v, want immediately right of the health bars %v", wedge, hp)
	}
	if capr.Min.X != hp.Min.X || capr.Max.X != wedge.Max.X {
		t.Errorf("capacitor %v, want the full column %d..%d", capr, hp.Min.X, wedge.Max.X)
	}
	if capr.Min.Y <= hp.Max.Y {
		t.Errorf("capacitor %v, want beneath the health/wedge band %v", capr, hp)
	}
	speed := h.speedLbl.GetWidget().Rect
	if speed.Max.X != capr.Max.X || speed.Min.Y < hp.Max.Y {
		t.Errorf("speed %v, want right-aligned under the wedge (column right edge %d)", speed, capr.Max.X)
	}
	if status.Dy() != 150 {
		t.Errorf("status height = %d, want 150 (the wedge's stack cost 223 before)", status.Dy())
	}
	row := h.targetRow.GetWidget().Rect
	if row.Min.Y < 9 || row.Min.Y > 11 {
		t.Errorf("target row top = %d, want 10 (top-centred)", row.Min.Y)
	}
	if want := (1280 - row.Dx()) / 2; row.Min.X < want-1 || row.Min.X > want+1 {
		t.Errorf("target row x = %d, want %d (centred)", row.Min.X, want)
	}

	c.Smoke(t)
}

// ---------------------------------------------------------------------------
// Design math: every piece of card/ring geometry is a pure function, pinned
// here with exact values.
// ---------------------------------------------------------------------------

// TestTargetCardGeometry pins the whole card layout at the default size and
// asserts the three layer bars tile the bars region with no gaps and no
// overlaps.
func TestTargetCardGeometry(t *testing.T) {
	card := image.Rect(0, 0, hudBoxW, hudBoxH) // 132×83
	if got := targetCardBody(card); got != image.Rect(4, 4, 128, 79) {
		t.Errorf("body = %v, want (4,4)-(128,79)", got)
	}
	if got := targetCardText(card); got != image.Rect(4, 4, 128, 25) {
		t.Errorf("name line = %v, want (4,4)-(128,25)", got)
	}
	if got := targetCardDist(card); got != image.Rect(4, 27, 128, 46) {
		t.Errorf("distance line = %v, want (4,27)-(128,46)", got)
	}
	if got := targetCardLock(card); got != image.Rect(4, 47, 128, 49) {
		t.Errorf("lock strip = %v, want (4,47)-(128,49)", got)
	}
	bars := targetCardBars(card)
	if bars != image.Rect(4, 51, 128, 79) {
		t.Fatalf("bars region = %v, want (4,51)-(128,79)", bars)
	}

	// The kind glyph box leads the name line: 16 px, centred in the line,
	// and the name text starts clear of it.
	ic := targetCardIcon(card)
	if ic != image.Rect(4, 6, 20, 22) {
		t.Errorf("icon box = %v, want (4,6)-(20,22)", ic)
	}
	name := targetCardName(card)
	if name != image.Rect(24, 4, 128, 25) {
		t.Errorf("name region = %v, want (24,4)-(128,25)", name)
	}
	if ic.Overlaps(name) {
		t.Errorf("icon %v overlaps the name region %v", ic, name)
	}
	if !ic.In(targetCardText(card)) {
		t.Errorf("icon %v escapes the name line %v", ic, targetCardText(card))
	}

	got := layerBars(bars)
	want := [3]image.Rectangle{
		image.Rect(4, 51, 128, 59),
		image.Rect(4, 61, 128, 69),
		image.Rect(4, 71, 128, 79),
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("layerBars[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	// Tiling invariants: full-width, stacked top-down, contiguous with the
	// 2 px gap, covering the region exactly — no gaps, no overlaps.
	for i, b := range got {
		if b.Min.X != bars.Min.X || b.Max.X != bars.Max.X {
			t.Errorf("bar %d spans x %d..%d, want %d..%d", i, b.Min.X, b.Max.X, bars.Min.X, bars.Max.X)
		}
		if i > 0 && b.Min.Y != got[i-1].Max.Y+hudBoxBarGap {
			t.Errorf("bar %d starts at %d, want %d (previous end + gap)",
				i, b.Min.Y, got[i-1].Max.Y+hudBoxBarGap)
		}
		if b.Dy() != got[0].Dy() && i < 2 {
			t.Errorf("bar %d height %d != bar 0 height %d", i, b.Dy(), got[0].Dy())
		}
	}
	if got[0].Min.Y != bars.Min.Y || got[2].Max.Y != bars.Max.Y {
		t.Errorf("bars cover %d..%d, want %d..%d", got[0].Min.Y, got[2].Max.Y, bars.Min.Y, bars.Max.Y)
	}
	// Odd heights still tile: the last bar absorbs the remainder.
	odd := layerBars(image.Rect(0, 0, 10, 30)) // (30-4)/3 = 8, remainder 2
	wantOdd := [3]image.Rectangle{
		image.Rect(0, 0, 10, 8),
		image.Rect(0, 10, 10, 18),
		image.Rect(0, 20, 10, 30),
	}
	if odd != wantOdd {
		t.Errorf("layerBars(10x30) = %v, want %v", odd, wantOdd)
	}
}

// TestBarFill pins the fraction→width mapping: exactly frac × width,
// clamped at both ends.
func TestBarFill(t *testing.T) {
	bar := image.Rect(0, 0, 100, 8)
	if got := barFill(bar, 0.37); got != image.Rect(0, 0, 37, 8) {
		t.Errorf("barFill(0.37) = %v, want (0,0)-(37,8) — exactly 0.37×100", got)
	}
	if got := barFill(bar, 0); got != image.Rect(0, 0, 0, 8) {
		t.Errorf("barFill(0) = %v, want empty", got)
	}
	if got := barFill(bar, 1.5); got != image.Rect(0, 0, 100, 8) {
		t.Errorf("barFill(1.5) = %v, want full", got)
	}
	if got := barFill(bar, -0.2); got != image.Rect(0, 0, 0, 8) {
		t.Errorf("barFill(-0.2) = %v, want empty", got)
	}
	// The real card bar: width 124, a quarter → 31 px exactly.
	cardBar := image.Rect(4, 46, 128, 54)
	if got := barFill(cardBar, 0.25); got != image.Rect(4, 46, 35, 54) {
		t.Errorf("barFill(card, 0.25) = %v, want (4,46)-(35,54)", got)
	}
}
