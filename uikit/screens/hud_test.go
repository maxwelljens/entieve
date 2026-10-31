package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/hulls"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
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
	wantName, wantDist := h.boxLabels(ship.Name, theme.FormatDistance(ship.Distance), 0)
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

// ---------------------------------------------------------------------------
// The owner's HUD tranche: WARP in the speed figure, click-to-select target
// cards, the modules cycling on a card, and the EWAR effect boxes.
// ---------------------------------------------------------------------------

// TestHudSpeedFigureReadsWARP: while a WarpDrive is live the speed figure is
// the literal WARP (any phase); the wedge keeps its clamped gauge, and with
// the drive gone the numeric figure comes back.
func TestHudSpeedFigureReadsWARP(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	s := env.Sim
	p := s.Player()
	h.Sync()
	if h.speedLbl.Label != theme.FormatSpeed(0) {
		t.Fatalf("at rest the figure = %q, want the numeric %q", h.speedLbl.Label, theme.FormatSpeed(0))
	}
	tr, ok := s.Transforms.Get(p)
	if !ok {
		t.Fatal("player has no transform")
	}
	s.Warps.Set(p, sim.WarpDrive{Phase: "align", To: tr.Pos.Add(core.V(1e7, 0))})
	h.Sync()
	if h.speedLbl.Label != "WARP" {
		t.Errorf("aligning figure = %q, want WARP", h.speedLbl.Label)
	}
	wd, _ := s.Warps.Get(p)
	wd.Phase = "warp"
	h.Sync()
	if h.speedLbl.Label != "WARP" {
		t.Errorf("warping figure = %q, want WARP", h.speedLbl.Label)
	}
	// The wedge keeps the clamped current/max gauge (not redesigned).
	m, ok := s.Motions.Get(p)
	if !ok {
		t.Fatal("player has no motion")
	}
	if m.MaxSpeed > 0 && h.wedgeFrac != hudFrac01(m.Vel.Len()/m.MaxSpeed) {
		t.Errorf("wedge = %v, want the clamped gauge %v", h.wedgeFrac, hudFrac01(m.Vel.Len()/m.MaxSpeed))
	}
	s.Warps.Remove(p)
	h.Sync()
	if h.speedLbl.Label != theme.FormatSpeed(m.Vel.Len()) {
		t.Errorf("after landing the figure = %q, want the numeric %q",
			h.speedLbl.Label, theme.FormatSpeed(m.Vel.Len()))
	}
}

// modView overrides the two module reads the target cards make (the fitting
// and each slot's activation target) over the real session's view: the sim
// still supplies the grid, the locks and every other read.
type modView struct {
	widgets.ContentView
	player  ecs.Entity
	fit     sim.Fitting
	targets map[sim.SlotRef]ecs.Entity
}

func (v *modView) Fitting(e ecs.Entity) (sim.Fitting, bool) {
	if e == v.player {
		return v.fit, true
	}
	return v.ContentView.Fitting(e)
}

func (v *modView) ModuleTarget(e ecs.Entity, slot sim.SlotRef) (ecs.Entity, bool) {
	if e == v.player {
		if tgt, ok := v.targets[slot]; ok {
			return tgt, true
		}
		return 0, false
	}
	return v.ContentView.ModuleTarget(e, slot)
}

// modEnv is the HUD's constructor param (Theme + ContentView) over that view.
type modEnv struct {
	theme.Theme
	widgets.ContentView
}

// TestHudTargetCardShowsCyclingModules: a card names the player's active
// modules whose activation target is that card's entity — module A only on
// card 1, module B only on card 2, nothing while they are off.
func TestHudTargetCardShowsCyclingModules(t *testing.T) {
	env, _ := newChromeEnv(t)
	s := env.Sim
	p := s.Player()
	h := NewHud(env)

	var one, two ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity == p {
			continue
		}
		if one == 0 {
			one = inf.Entity
		} else if two == 0 {
			two = inf.Entity
			break
		}
	}
	if one == 0 || two == 0 {
		t.Fatal("fixture: showcase grid has no two entities to lock")
	}
	tg, ok := s.Targetings.Get(p)
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{{Target: one, Progress: 1}, {Target: two, Progress: 1}}

	// The player's two EWAR modules, both cycling, one per target.
	view := widgets.ContentView(env)
	mv := &modView{
		ContentView: view,
		player:      p,
		fit: sim.Fitting{
			Mid:   []content.ModuleID{"mod-stasis-web", "mod-warp-scram"},
			MidRT: []sim.ModuleRuntime{{Def: "mod-stasis-web", Active: true}, {Def: "mod-warp-scram", Active: true}},
		},
		targets: map[sim.SlotRef]ecs.Entity{
			{Class: content.SlotMid, Index: 0}: one,
			{Class: content.SlotMid, Index: 1}: two,
		},
	}
	h2 := NewHud(modEnv{Theme: env, ContentView: mv})
	h2.Sync()

	if h2.boxes[0].card.nmods != 1 || h2.boxes[0].card.mods[0] != "mod-stasis-web" {
		t.Errorf("card 1 mods = %v (n=%d), want the web module's own glyph only",
			h2.boxes[0].card.mods, h2.boxes[0].card.nmods)
	}
	if h2.boxes[1].card.nmods != 1 || h2.boxes[1].card.mods[0] != "mod-warp-scram" {
		t.Errorf("card 2 mods = %v (n=%d), want the scram module's own glyph only",
			h2.boxes[1].card.mods, h2.boxes[1].card.nmods)
	}
	// Idle modules carry no glyph.
	for i := range mv.fit.MidRT {
		mv.fit.MidRT[i].Active = false
	}
	h2.Sync()
	for i := 0; i < 2; i++ {
		if n := h2.boxes[i].card.nmods; n != 0 {
			t.Errorf("idle card %d mods n = %d, want none", i, n)
		}
	}
	// The glyphs sit on the distance line, inside the card, and clear of
	// the distance text's ellipsis reserve.
	card := image.Rect(0, 0, hudBoxW, hudBoxH)
	for i := 0; i < hudBoxMods; i++ {
		box := targetCardMod(card, i)
		if !box.In(targetCardDist(card)) {
			t.Errorf("mod box %d = %v, want it on the distance line %v", i, box, targetCardDist(card))
		}
	}
	if got, want := cardModsWidth(2), 2*theme.IconInline+theme.IconGap; got != want {
		t.Errorf("cardModsWidth(2) = %d, want %d", got, want)
	}
	if h.boxes[0].card.nmods != 0 {
		t.Error("the un-wired HUD grew module glyphs")
	}
}

// TestHudTargetCardClickSelects drives a real press/release on a laid-out
// card through the widget pipeline: the card's entity is reported to the
// selector (the game wires Kit.Select). Empty chrome between cards and a
// hidden target row select nothing.
func TestHudTargetCardClickSelects(t *testing.T) {
	fm := useFakeMouse(t)
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)

	var picked []ecs.Entity
	h.SetSelector(func(e ecs.Entity) { picked = append(picked, e) })

	s := env.Sim
	p := s.Player()
	var target ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity != p {
			target = inf.Entity
			break
		}
	}
	if target == 0 {
		t.Fatal("fixture: no entity to lock")
	}
	tg, ok := s.Targetings.Get(p)
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{{Target: target, Progress: 1}}
	c.Frame(t, 1280, 720)

	box := h.boxes[0].root.GetWidget().Rect
	if box.Empty() {
		t.Fatal("fixture: card never laid out")
	}
	pt := image.Pt((box.Min.X+box.Max.X)/2, (box.Min.Y+box.Max.Y)/2)
	click := func(at image.Point) {
		fm.at(at.X, at.Y)
		c.Frame(t, 1280, 720) // hover settles
		fm.down()
		c.Frame(t, 1280, 720)
		fm.up()
		c.Frame(t, 1280, 720) // the click fires on the release
	}

	click(pt)
	if len(picked) != 1 || picked[0] != target {
		t.Fatalf("card click reported %v, want [%v]", picked, target)
	}

	// Empty chrome: left of the centred card row, inside nothing.
	click(image.Pt(box.Min.X-20, pt.Y))
	if len(picked) != 1 {
		t.Errorf("a click on empty chrome selected %v", picked[1:])
	}

	// Hidden row: no card, no selection.
	tg.Locks = nil
	c.Frame(t, 1280, 720)
	click(pt)
	if len(picked) != 1 {
		t.Errorf("a click on the hidden target row selected %v", picked[1:])
	}
}

// TestHudEffectBoxes: web and scram each get a box above the ship status
// while their sim expiry is ahead, with the time left; both vanish at expiry
// and the row hides.
func TestHudEffectBoxes(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	s := env.Sim
	p := s.Player()

	h.Sync()
	if vis := h.effects.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Fatalf("no effects but the effect row is visible (visibility %d)", vis)
	}
	for i, e := range h.eff {
		if vis := e.root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
			t.Fatalf("effect box %d visible with no effect", i)
		}
	}

	m, ok := s.Motions.Get(p)
	if !ok {
		t.Fatal("player has no motion")
	}
	sh, ok := s.Ships.Get(p)
	if !ok {
		t.Fatal("player has no ship")
	}
	now := s.Time()
	m.WebFactor, m.WebbedUntil = 0.55, now+7
	sh.ScrambledUntil = now + 3
	h.Sync()

	if vis := h.effects.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Fatalf("effects live but the row is hidden")
	}
	for i, want := range []string{"7.0 s", "3.0 s"} {
		if vis := h.eff[i].root.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
			t.Errorf("effect box %d hidden while its effect is live", i)
		}
		if got := h.eff[i].lbl.Label; got != want {
			t.Errorf("effect box %d remaining = %q, want %q", i, got, want)
		}
	}

	// The row sits directly above the ship status (strip, effects, status).
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.Frame(t, 1280, 720)
	strip, effs, status := h.strip.GetWidget().Rect, h.effects.GetWidget().Rect, h.status.GetWidget().Rect
	if effs.Max.Y > status.Min.Y || effs.Min.Y < strip.Max.Y {
		t.Errorf("effects %v not between the strip %v and the status %v", effs, strip, status)
	}

	// Expiry clears both boxes and hides the row again.
	s.Step()
	m.WebbedUntil = s.Time() - 1
	sh.ScrambledUntil = s.Time() - 1
	h.Sync()
	for i, e := range h.eff {
		if vis := e.root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
			t.Errorf("effect box %d still visible after expiry", i)
		}
	}
	if vis := h.effects.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("the effect row stays visible after expiry")
	}
}
