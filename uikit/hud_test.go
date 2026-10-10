package uikit

import (
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// newChromeEnv builds the environment shared by the chrome tests: the
// embedded content, world-showcase at seed 42 and a command-capturing
// enqueue. (kit_test's newTestKit drives the whole Kit instead.)
func newChromeEnv(t *testing.T) (Env, *[]sim.Command) {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	var cmds []sim.Command
	env := NewEnv(c, s, func(cmd sim.Command) { cmds = append(cmds, cmd) })
	return env, &cmds
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
		ui:   &ebitenui.UI{Container: root, PrimaryTheme: env.Theme},
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
	// structure, cap 1800/1800 GJ, wallet 10,000,000 cr, speed 0.
	for _, c := range []struct{ name, got, want string }{
		{"shield", h.shieldLbl.Label, "Shield 500/500"},
		{"armor", h.armorLbl.Label, "Armor 500/500"},
		{"structure", h.structLbl.Label, "Structure 600/600"},
		{"speed", h.speedLbl.Label, formatSpeed(0)},
		{"time", h.timeLbl.Label, "×1 time"},
		{"wallet", h.walletLbl.Label, "10,000,000.00 cr"},
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

	// Wallet formatting (incl. the negative and carry cases).
	for _, c := range []struct {
		in   float64
		want string
	}{
		{-1234.5, "-1,234.50 cr"},
		{0.999, "1.00 cr"},
		{0, "0.00 cr"},
	} {
		if got := h.creditsLabel(c.in); got != c.want {
			t.Errorf("creditsLabel(%v) = %q, want %q", c.in, got, c.want)
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
// one box per lock with name, lock progress and HP mini-bars, hidden again
// when the locks drop.
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
	if b0.name.Label != Ellipsize(ship.Name, hudBoxW-2*contentsPad, h.measure) {
		t.Errorf("box 0 name = %q, want %q", b0.name.Label, ship.Name)
	}
	if got, want := b0.lock.GetCurrent(), 420; got != want {
		t.Errorf("box 0 lock = %d, want %d", got, want)
	}
	// A ship target reports its three layers; the asteroid reports the read
	// model's single health fraction in the first bar.
	if sh, ok := s.Ships.Get(ship.Entity); ok {
		want := int(hudFrac(sh.Shield, sh.MaxShield) * hudCapMax)
		if b0.layer[0].GetCurrent() != want {
			t.Errorf("box 0 shield bar = %d, want %d", b0.layer[0].GetCurrent(), want)
		}
	}
	if b1.lock.GetCurrent() != hudCapMax {
		t.Errorf("box 1 lock = %d, want full", b1.lock.GetCurrent())
	}
	if b1.layer[0].GetCurrent() != int(rock.Health*hudCapMax) {
		t.Errorf("box 1 health bar = %d, want %d", b1.layer[0].GetCurrent(), int(rock.Health*hudCapMax))
	}
	if vis := b2.root.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
		t.Errorf("third box visible with two locks (visibility %d)", vis)
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
