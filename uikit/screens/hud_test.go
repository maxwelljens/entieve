package screens

import (
	"image"
	"math"
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
	if b := h.portrait.Bounds(); b.Dx() != hudPortraitW || b.Dy() != hudPortraitH {
		t.Fatalf("portrait image %v, want %dx%d", b, hudPortraitW, hudPortraitH)
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
// hidden again when the locks drop (the row itself stays mounted: all cards
// hidden lays it out 0×0, the top chrome's empty state). The card state is
// the render input, so pinning it pins what gets drawn.
func TestHudTargetBoxes(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	s := env.Sim
	player := s.Player()
	h.Sync()
	if vis := h.boxes[0].root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Fatalf("no locks but card 0 visibility = %d", vis)
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
	if vis := h.boxes[0].root.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Fatalf("locked targets hide card 0 (visibility %d)", vis)
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

	// A target that is gone blinks before it disappears: the ghost keeps the
	// last card, alternates on the frame count and is gone after the window.
	// A click never lands on it (live stays false).
	tg.Locks = nil
	h.Sync()
	if b0.live {
		t.Error("the ghost card still answers clicks (live)")
	}
	if vis := b0.root.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Fatalf("the lost lock left no ghost (visibility %d)", vis)
	}
	flips, dark := 0, false
	wasOn := true
	for i := 0; i < hudGhostFrames; i++ {
		h.Sync()
		on := b0.root.GetWidget().GetVisibility() == widget.Visibility_Show
		if on != wasOn {
			flips++
		}
		if !on {
			dark = true
		}
		wasOn = on
	}
	if flips < 2 || !dark {
		t.Errorf("the ghost blinked %d times (dark phase seen: %v), want an alternating blink", flips, dark)
	}
	if vis := b0.root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("the ghost outlived its window (visibility %d)", vis)
	}
	if !b0.live && b0.ghost != 0 {
		t.Errorf("the blink counter = %d after the window, want 0", b0.ghost)
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
	c.root.AddChild(h.Cards().Container()) // the game mounts it as top chrome
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

	// The pair centres in the band the log's pocket leaves free (the
	// left padding carries the log's 264 px width), not on the viewport: a
	// viewport-centred 600 px pair would start at 340, 21 px inside the log.
	pair := h.pair.GetWidget().Rect
	status := h.status.GetWidget().Rect
	strip := h.strip.GetWidget().Rect
	rack := h.pair.Children()[1].GetWidget().Rect
	if pair.Max.Y < 709 || pair.Max.Y > 711 {
		t.Errorf("pair bottom = %d, want 710 (bottom-centred)", pair.Max.Y)
	}
	band := theme.Px(10 + LogDefaultW)
	if want := band + (1280-band-theme.Px(10)-pair.Dx())/2; pair.Min.X < want-1 || pair.Min.X > want+1 {
		t.Errorf("pair x = %d, want %d (centred right of the log pocket)", pair.Min.X, want)
	}
	if pair.Min.X <= 361 {
		t.Errorf("pair left = %d, want clear of the log pocket (right edge 361)", pair.Min.X)
	}
	if status.Max.Y != pair.Max.Y {
		t.Errorf("status bottom = %d, want the pair baseline %d", status.Max.Y, pair.Max.Y)
	}
	// The portrait is the owner's 240×60 box: a quarter of its width tall,
	// the frame its drop-in art (~4.8:1 ink) fills width-bound without
	// distortion (contain, hullart.go).
	if p := h.portraitGfx.GetWidget().Rect; p.Dx() != theme.Px(hudPortraitW) || p.Dy() != theme.Px(hudPortraitH) {
		t.Errorf("portrait box = %v, want the %d×%d box", p, theme.Px(hudPortraitW), theme.Px(hudPortraitH))
	} else if p.Dx() != 4*p.Dy() {
		t.Errorf("portrait box %v is not 4:1 wide", p)
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

// modView overrides the module reads the target cards make (the fitting, each
// slot's activation target, the sim clock the rings are read against and the
// bay a loading ring needs) over the real session's view: the sim still
// supplies the grid, the locks and every other read.
type modView struct {
	widgets.ContentView
	player  ecs.Entity
	fit     sim.Fitting
	targets map[sim.SlotRef]ecs.Entity
	now     float64                       // clock override (0 = the sim's own)
	clips   map[sim.SlotRef]sim.ClipState // bay override
}

func (v *modView) Fitting(e ecs.Entity) (sim.Fitting, bool) {
	if e == v.player {
		return v.fit, true
	}
	return v.ContentView.Fitting(e)
}

func (v *modView) Time() float64 {
	if v.now != 0 {
		return v.now
	}
	return v.ContentView.Time()
}

func (v *modView) Clip(slot sim.SlotRef) (sim.ClipState, bool) {
	if c, ok := v.clips[slot]; ok {
		return c, true
	}
	return v.ContentView.Clip(slot)
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

	if h2.boxes[0].card.nmods != 1 || h2.boxes[0].card.mods[0].glyph != "mod-stasis-web" {
		t.Errorf("card 1 mods = %v (n=%d), want the web module's own glyph only",
			h2.boxes[0].card.mods, h2.boxes[0].card.nmods)
	}
	if h2.boxes[1].card.nmods != 1 || h2.boxes[1].card.mods[0].glyph != "mod-warp-scram" {
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
	if got, want := cardModsWidth(2), 2*theme.Px(hudBoxModCell)+theme.Px(theme.IconGap); got != want {
		t.Errorf("cardModsWidth(2) = %d, want %d", got, want)
	}
	if h.boxes[0].card.nmods != 0 {
		t.Error("the un-wired HUD grew module glyphs")
	}
}

// TestHudTargetCardModuleRing is the card mini-cell pin: an active module on
// a locked target draws the rack's cycle ring, depleting with the sim clock
// (two pumps at different times differ), greyed while a cancelled cycle runs
// out, lit while a bay loads and gone once the clock is spent. The cap and
// the distance text's clearance rule are unchanged.
func TestHudTargetCardModuleRing(t *testing.T) {
	env, _ := newChromeEnv(t)
	s := env.Sim
	p := s.Player()
	var tgt ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity != p {
			tgt = inf.Entity
			break
		}
	}
	if tgt == 0 {
		t.Fatal("fixture: no entity to lock")
	}
	tg, _ := s.Targetings.Get(p)
	tg.Locks = []sim.Lock{{Target: tgt, Progress: 1}}

	def, ok := env.Defs().Module("mod-stasis-web")
	if !ok || def.ActivationTime <= 0 {
		t.Fatal("fixture: the stasis web has no activation time")
	}
	web := sim.SlotRef{Class: content.SlotMid, Index: 0}
	mv := &modView{
		ContentView: env, player: p, now: 100,
		fit: sim.Fitting{
			Mid: []content.ModuleID{"mod-stasis-web"},
			MidRT: []sim.ModuleRuntime{{Def: "mod-stasis-web", Active: true,
				CycleEnds: 100 + 0.75*def.ActivationTime}},
		},
		targets: map[sim.SlotRef]ecs.Entity{web: tgt},
	}
	h := NewHud(modEnv{Theme: env, ContentView: mv})
	h.Sync()

	cell := h.boxes[0].card.mods[0]
	if cell.glyph != "mod-stasis-web" {
		t.Fatalf("card cell glyph = %q, want the web module's", cell.glyph)
	}
	if math.Abs(cell.ring-0.75) > 0.01 {
		t.Errorf("ring remaining = %v, want 0.75 (three quarters of the cycle left)", cell.ring)
	}
	if !cell.lit {
		t.Error("an active module's ring is not lit")
	}

	// The same cycle one second later: the ring has depleted.
	mv.now = 101
	h.Sync()
	next := h.boxes[0].card.mods[0].ring
	if next >= cell.ring {
		t.Errorf("ring at t+1 s = %v, want less than %v (it must advance with CycleEnds)", next, cell.ring)
	}

	// Cancelled mid-cycle: the ring still shows, greyed (the rack's rule).
	mv.fit.MidRT[0].Active = false
	mv.fit.MidRT[0].CycleEnds = mv.now + 2
	h.Sync()
	cell = h.boxes[0].card.mods[0]
	if cell.ring < 0 || cell.lit {
		t.Errorf("cancelled cell = %+v, want a greyed ring (ring >= 0, lit false)", cell)
	}
	// Spent clock, not loading: no cell at all.
	mv.now += 3
	h.Sync()
	if n := h.boxes[0].card.nmods; n != 0 {
		t.Errorf("a spent cycle still draws %d cell(s), want none", n)
	}

	// A loading bay draws a lit ring (activity, not a cancellation).
	mv.fit.MidRT[0].CycleEnds = mv.now + 1
	mv.clips = map[sim.SlotRef]sim.ClipState{web: {Loading: true}}
	h.Sync()
	cell = h.boxes[0].card.mods[0]
	if cell.ring < 0 || !cell.lit {
		t.Errorf("loading cell = %+v, want a lit ring", cell)
	}

	// Cap: four bound slots show only hudBoxMods cells, and the distance
	// text stays clear of the room they take.
	mv.fit.Mid = []content.ModuleID{"mod-stasis-web", "mod-stasis-web", "mod-stasis-web", "mod-stasis-web"}
	mv.fit.MidRT = make([]sim.ModuleRuntime, 4)
	for i := range mv.fit.MidRT {
		mv.fit.MidRT[i] = sim.ModuleRuntime{Def: "mod-stasis-web", Active: true, CycleEnds: mv.now + 1}
		mv.targets[sim.SlotRef{Class: content.SlotMid, Index: i}] = tgt
	}
	h.Sync()
	if n := h.boxes[0].card.nmods; n != hudBoxMods {
		t.Errorf("card cells = %d, want the cap of %d", n, hudBoxMods)
	}
	card := image.Rect(0, 0, hudBoxW, hudBoxH)
	room := float64(targetCardDist(card).Dx() - cardModsWidth(hudBoxMods))
	_, dist := h.boxLabels("", theme.FormatDistance(9_999_999), hudBoxMods)
	if w := h.measureTiny(dist); w > room {
		t.Errorf("distance text %q is %v px wide, want it ellipsized to %v", dist, w, room)
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
	c.root.AddChild(h.Cards().Container()) // the game mounts it as top chrome

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

// TestHudTargetCardsOverOverview pins the locked-target row as TOP chrome.
// The default Overview covers the top-centre band (its 760×409 default lands
// at 95,8..855,417 at 1024, the one-lock row 446..578 × 10..93 inside it),
// so a row mounted with the HUD's under-window container is dead there: the
// window owns the band and eats the press. Through Kit.AddTopContainer the
// card's centre answers LayerTopChrome and the click selects.
//
// Three more rules ride along: the gap between two cards is claimed by the
// column (top layer, no control — the overview under it keeps its clicks),
// an overview row outside the band selects as always, and with no locks the
// row lays out empty so the band falls back to the window.
func TestHudTargetCardsOverOverview(t *testing.T) {
	fm := useFakeMouse(t)
	k, _ := newTestKitWith(t, widgets.NewPointer(fm))
	h := NewHud(k.Env())
	k.AddContainer(h.Container(), widget.AnchorLayoutData{
		StretchHorizontal: true, StretchVertical: true,
	})
	k.AddTopContainer(h.Cards(), widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionCenter,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		Padding:            &widget.Insets{Top: theme.Px(10)},
	})
	var picked []ecs.Entity
	h.SetSelector(func(e ecs.Entity) { picked = append(picked, e) })

	s := k.Sim
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
	frame := func() {
		h.Sync()
		pumpKit(t, k, 1024, 700)
	}
	frame()

	box := h.boxes[0].root.GetWidget().Rect
	if box.Empty() {
		t.Fatal("fixture: card never laid out")
	}
	pt := mid(box)
	ov := k.OverviewWindow().GetContainer().GetWidget().Rect
	if !pt.In(ov) {
		t.Fatalf("fixture: card centre %v is not under the overview %v", pt, ov)
	}
	if o := k.Own(pt); o.Layer != LayerTopChrome {
		t.Errorf("card centre %v = layer %d (window %v), want layer %d (top chrome)",
			pt, o.Layer, o.Window, LayerTopChrome)
	}

	click := func(at image.Point) {
		fm.at(at.X, at.Y)
		frame() // hover settles
		fm.down()
		frame()
		fm.up()
		frame() // the click fires on the release
	}
	click(pt)
	if len(picked) != 1 || picked[0] != target {
		t.Fatalf("card click over the overview reported %v, want [%v]", picked, target)
	}

	// Two cards: the gap between them is the column's — the top layer owns
	// it with no control, and the overview row underneath never sees the
	// press (BlockLower). Disclosed limit: while locks exist the overview
	// keeps no clicks on the row's band, gaps included.
	var others []ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity != p && inf.Entity != target {
			others = append(others, inf.Entity)
		}
		if len(others) == 1 {
			break
		}
	}
	if len(others) == 0 {
		t.Fatal("fixture: no second entity to lock")
	}
	tg.Locks = []sim.Lock{{Target: target, Progress: 1}, {Target: others[0], Progress: 1}}
	frame()
	// The row re-centres on two cards: read both fresh.
	c0, c1 := h.boxes[0].root.GetWidget().Rect, h.boxes[1].root.GetWidget().Rect
	gap := image.Pt((c0.Max.X+c1.Min.X)/2, mid(c1).Y)
	if gap.X <= c0.Max.X || gap.X >= c1.Min.X {
		t.Fatalf("fixture: gap %v is not between the cards %v and %v", gap, c0, c1)
	}
	if o := k.Own(gap); o.Layer != LayerTopChrome || o.Control {
		t.Errorf("gap %v = layer %d control %v, want the card column (layer %d), no control",
			gap, o.Layer, o.Control, LayerTopChrome)
	}
	click(gap)
	if len(picked) != 1 {
		t.Errorf("a gap click selected cards %v", picked[1:])
	}
	if got := k.Selected(); got != 0 {
		t.Errorf("a gap click reached the overview underneath: selection = %v", got)
	}

	// An overview row below the band is the window's, as always.
	row := image.Rectangle{}
	for i, n := 0, len(k.OverviewTable().RowsForTest()); i < n; i++ {
		if r := k.OverviewTable().RowWidgetForTest(i).Rect; r.Min.Y > box.Max.Y+4 && !r.Empty() {
			row = r
			break
		}
	}
	if row.Empty() {
		t.Fatal("fixture: no overview row below the card band")
	}
	rowPt := mid(row)
	if o := k.Own(rowPt); o.Layer != LayerWindow || o.Window != k.OverviewWindow() {
		t.Errorf("overview row below the band %v = layer %d window %v, want the overview (layer %d)",
			rowPt, o.Layer, o.Window, LayerWindow)
	}
	click(rowPt)
	if k.Selected() == 0 {
		t.Error("a click on an overview row below the band selected nothing")
	}

	// No locks: the ghost blinks out (1.5 s, frame-counted), then the row
	// lays out empty and the band is the overview's again — in the walk AND
	// in the real input path (the parked top layer blocks nothing there).
	tg.Locks = nil
	for i := 0; i <= hudGhostFrames; i++ {
		frame()
	}
	if o := k.Own(pt); o.Layer != LayerWindow || o.Window != k.OverviewWindow() {
		t.Errorf("no locks: card band %v = layer %d window %v, want the overview (layer %d)",
			pt, o.Layer, o.Window, LayerWindow)
	}
	// ... and in the real input path: a window control sitting where the cards
	// were takes the click (the parked top layer blocks nothing).
	clicked := 0
	btn := widget.NewButton(widget.ButtonOpts.TextLabel("x"),
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MouseButtonClickedHandler(
			func(*widget.WidgetMouseButtonClickedEventArgs) { clicked++ })))
	body := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	body.AddChild(btn)
	k.AddWindow(widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.MinSize(theme.Px(240), theme.Px(120)),
		widget.WindowOpts.Location(box),
	))
	frame()
	bp := mid(btn.GetWidget().Rect)
	if !bp.In(box) {
		t.Fatalf("fixture: window button %v is not inside the old card band %v", bp, box)
	}
	click(bp)
	if clicked != 1 {
		t.Errorf("no locks: a click on a window control at %v inside the band fired %d times, want 1", bp, clicked)
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

// TestHudTargetCardSelectionBorder: the locked card of the player's current
// selection stages selected=true and repaints when the selection moves, so
// the same border follows an overview-row selection, a world click and a
// card click (all three land on Kit.Selected, the picker's source).
func TestHudTargetCardSelectionBorder(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.root.AddChild(h.Cards().Container())

	s := env.Sim
	p := s.Player()
	var ship, rock ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity == p {
			continue
		}
		if inf.Kind == sim.KindShip && ship == 0 {
			ship = inf.Entity
		} else if inf.Kind == sim.KindAsteroid && rock == 0 {
			rock = inf.Entity
		}
	}
	if ship == 0 || rock == 0 {
		t.Fatalf("showcase grid missing ship/rock (ship=%v rock=%v)", ship, rock)
	}
	tg, _ := s.Targetings.Get(p)
	tg.Locks = []sim.Lock{{Target: ship, Progress: 1}, {Target: rock, Progress: 1}}

	var selected ecs.Entity
	h.SetTargetPicker(func() ecs.Entity { return selected })
	c.Frame(t, 1280, 720)
	if h.boxes[0].card.selected || h.boxes[1].card.selected {
		t.Fatal("an empty selection lit a card border")
	}

	before := h.boxes[1].draws
	selected = rock
	c.Frame(t, 1280, 720)
	if h.boxes[0].card.selected {
		t.Fatal("the unselected card lit its border")
	}
	if !h.boxes[1].card.selected {
		t.Fatal("the selected target's card did not stage its border")
	}
	if h.boxes[1].drawn != h.boxes[1].card || h.boxes[1].draws != before+1 {
		t.Fatalf("selected card repaints = %d -> %d, want one repaint carrying selected=true",
			before, h.boxes[1].draws)
	}

	// Deselecting returns the card to the plain hairline.
	selected = 0
	c.Frame(t, 1280, 720)
	if h.boxes[1].card.selected {
		t.Fatal("deselecting left the selection border on")
	}
}
