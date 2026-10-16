package uikit

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// newChromeEnv builds the environment shared by the chrome tests: the
// embedded content, world-showcase at seed 42 and a command-capturing
// enqueue. (kit_test's newTestKit drives the whole Kit instead.)
func newChromeEnv(t *testing.T) (*testEnv, *[]sim.Command) {
	t.Helper()
	return newTestEnv(t)
}

// chromeUI is the shared chrome test harness: an interface root (anchor
// layout) holding the anchored panels plus any windows, driven through
// Sync → Update → Draw exactly the way the game will drive it.
type chromeUI struct {
	env    Env
	ui     *ebitenui.UI
	root   *widget.Container
	panels []Panel
	wins   []*widget.Window
}

func newChromeUI(env Env) *chromeUI {
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	return &chromeUI{
		env:  env,
		ui:   &ebitenui.UI{Container: root, PrimaryTheme: env.WidgetTheme()},
		root: root,
	}
}

// addPanel attaches an anchored panel's container and its Sync target.
func (c *chromeUI) addPanel(container *widget.Container, p Panel) *chromeUI {
	c.root.AddChild(container)
	c.panels = append(c.panels, p)
	return c
}

// addWindow adds a window panel to the UI.
func (c *chromeUI) addWindow(w WindowPanel) *chromeUI {
	c.ui.AddWindow(w.Win())
	c.panels = append(c.panels, w)
	c.wins = append(c.wins, w.Win())
	return c
}

// update runs one UI update (this is what flushes ebitenui's deferred
// event queue: handler registration, event delivery).
func (c *chromeUI) update(t *testing.T) {
	t.Helper()
	c.ui.Update()
}

// frame runs one full Sync/Update/Draw cycle at w×h.
func (c *chromeUI) frame(t *testing.T, w, h int) *ebiten.Image {
	t.Helper()
	img := ebiten.NewImage(w, h)
	for _, p := range c.panels {
		p.Sync()
	}
	c.ui.Update()
	c.ui.Draw(img)
	return img
}

// smoke draws two frames at both design sizes and asserts every window
// stays inside the screen.
func (c *chromeUI) smoke(t *testing.T) {
	t.Helper()
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		c.frame(t, size[0], size[1])
		c.frame(t, size[0], size[1])
		for _, w := range c.wins {
			r := w.GetContainer().GetWidget().Rect
			if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > size[0] || r.Max.Y > size[1] {
				t.Errorf("%dx%d: window %v out of bounds", size[0], size[1], r)
			}
		}
	}
}

// TestHudConstruction checks the panel builds and Sync fills it from the
// showcase sim without panicking.
func TestHudConstruction(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	if h.Container() == nil {
		t.Fatal("Hud.Container() is nil")
	}
	if h.shieldLbl.Label != "" {
		t.Errorf("shield label before Sync = %q, want empty", h.shieldLbl.Label)
	}
	h.Sync()
	if h.shieldLbl.Label == "" {
		t.Error("Sync did not populate the shield label")
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
		{"shield", h.shieldLbl.Label, "Shield 500/500"},
		{"armor", h.armorLbl.Label, "Armor 500/500"},
		{"structure", h.structLbl.Label, "Structure 600/600"},
		{"speed", h.speedLbl.Label, formatSpeed(0)},
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
		if got := FormatCredits(c.in); got != c.want {
			t.Errorf("FormatCredits(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHudRingRedrawsOnDamage pins the arc image to the damage state: the
// ring repaints when a layer fraction moves and stays untouched otherwise.
func TestHudRingRedrawsOnDamage(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	h.Sync()
	if h.ringFrac != [3]float64{1, 1, 1} {
		t.Fatalf("ring fractions = %v, want all full", h.ringFrac)
	}
	if h.redraws != 1 {
		t.Fatalf("first Sync repainted %d times, want 1", h.redraws)
	}
	if b := h.ring.Bounds(); b.Dx() != hudRingSize || b.Dy() != hudRingSize {
		t.Fatalf("ring image %v, want %dx%d", b, hudRingSize, hudRingSize)
	}

	h.Sync() // nothing changed → no repaint
	if h.redraws != 1 {
		t.Errorf("idle Sync repainted the ring (%d)", h.redraws)
	}

	sh, ok := env.Sim.Ships.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no ship")
	}
	sh.Shield = 125 // 25 %
	h.Sync()
	if h.ringFrac[0] != 0.25 {
		t.Errorf("shield fraction = %v, want 0.25", h.ringFrac[0])
	}
	if h.redraws != 2 {
		t.Errorf("damage repainted %d times, want 2", h.redraws)
	}
	if h.shieldLbl.Label != "Shield 125/500" {
		t.Errorf("shield label = %q", h.shieldLbl.Label)
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
	wantName, wantDist := h.boxLabels(ship.Name, formatDistance(ship.Distance))
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

// TestHudDrawSmoke draws the HUD at both design sizes inside an interface
// root and checks the anchor contract: bar bottom-center, target row
// top-center.
func TestHudDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	c := newChromeUI(env)
	c.addPanel(h.Container(), h)
	c.frame(t, 1280, 720)

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
	c.frame(t, 1280, 720)

	bar := h.bar.GetWidget().Rect
	if bar.Max.Y < 709 || bar.Max.Y > 711 {
		t.Errorf("bar bottom = %d, want 710 (bottom-centred)", bar.Max.Y)
	}
	if want := (1280 - bar.Dx()) / 2; bar.Min.X < want-1 || bar.Min.X > want+1 {
		t.Errorf("bar x = %d, want %d (centred)", bar.Min.X, want)
	}
	row := h.targetRow.GetWidget().Rect
	if row.Min.Y < 9 || row.Min.Y > 11 {
		t.Errorf("target row top = %d, want 10 (top-centred)", row.Min.Y)
	}
	if want := (1280 - row.Dx()) / 2; row.Min.X < want-1 || row.Min.X > want+1 {
		t.Errorf("target row x = %d, want %d (centred)", row.Min.X, want)
	}

	c.smoke(t)
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

// TestRingSweepAndTicks pins the ring math: sweep at 0/25/50/100 % is
// 0/quarter/half/full, ticks sit at the quarter marks, and the depleted
// track dims in RGB (never alpha).
func TestRingSweepAndTicks(t *testing.T) {
	for _, c := range []struct {
		frac float64
		want float64
	}{
		{0, 0},
		{0.25, math.Pi / 2},
		{0.5, math.Pi},
		{1, 2 * math.Pi},
		{1.7, 2 * math.Pi}, // clamped
		{-3, 0},            // clamped
	} {
		if got := ringSweep(c.frac); got != c.want {
			t.Errorf("ringSweep(%v) = %v, want %v", c.frac, got, c.want)
		}
	}
	ticks := ringTicks()
	want := [3]float64{math.Pi / 2, math.Pi, ringSweep(0.75)}
	for i := range want {
		if ticks[i] != want[i] {
			t.Errorf("ringTicks()[%d] = %v, want %v", i, ticks[i], want[i])
		}
	}
	for i := 1; i < len(ticks); i++ {
		if d := ticks[i] - ticks[i-1]; math.Abs(d-math.Pi/2) > 1e-12 {
			t.Errorf("tick spacing %d = %v, want π/2", i, d)
		}
	}

	// The dim track: layer color scaled to 25 % in RGB at full alpha.
	for _, c := range []struct {
		name string
		in   color.RGBA
		want color.RGBA
	}{
		{"shield", color.RGBA{111, 168, 255, 255}, color.RGBA{27, 42, 63, 255}},
		{"armor", color.RGBA{224, 163, 60, 255}, color.RGBA{56, 40, 15, 255}},
		{"hull", color.RGBA{201, 206, 214, 255}, color.RGBA{50, 51, 53, 255}},
	} {
		got := ringTrackColor(c.in)
		if got != c.want {
			t.Errorf("ringTrackColor(%s) = %v, want %v", c.name, got, c.want)
		}
		if got.A != 255 {
			t.Errorf("ringTrackColor(%s) alpha = %d, want 255 (dim in RGB, not alpha)", c.name, got.A)
		}
	}
}
