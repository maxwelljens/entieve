package screens

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// fitTileAt returns the tile showing one slot (nil when the hull hides it).
func fitTileAt(f *Fitting, slot sim.SlotRef) *fitTile {
	for row := range fitClasses {
		if fitClasses[row] != slot.Class {
			continue
		}
		if slot.Index < 0 || slot.Index >= fitMaxCells {
			return nil
		}
		return f.tiles[row][slot.Index]
	}
	return nil
}

// fitTileShown reports whether a tile is part of the hull's shape.
func fitTileShown(t *fitTile) bool {
	return t.root.GetWidget().GetVisibility() == widget.Visibility_Show
}

// clickFitTile fires one tile's left-button click and flushes the queue.
func clickFitTile(ui *ebitenui.UI, t *fitTile) {
	w := t.btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: w,
		Button: ebiten.MouseButtonLeft,
	})
	ui.Update()
}

// rightClickFitTile opens one tile's context menu at the cursor.
func rightClickFitTile(t *testing.T, ui *ebitenui.UI, tile *fitTile) {
	t.Helper()
	w := tile.btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 12,
		OffsetY: 6,
	})
	ui.Update()
}

// TestFittingRowTooltips pins the Fitting window's tooltips to the rack's
// builder: a filled slot tile carries the same structured panel (slot in
// the sub line, no rack-only hints), an empty slot tile carries none, and
// the hangar's not-yet-fitted modules get the slotless variant while
// stored hulls stay bare.
func TestFittingRowTooltips(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	fitted := -1
	empty := -1
	for row := range f.tiles {
		for idx, tile := range f.tiles[row] {
			if !fitTileShown(tile) {
				continue
			}
			if tile.mod != "" && fitted < 0 {
				fitted = row*fitMaxCells + idx
			}
			if tile.mod == "" && tile.slot.Class == content.SlotHigh && empty < 0 {
				empty = row*fitMaxCells + idx
			}
		}
	}
	if fitted < 0 {
		t.Fatal("fixture: no fitted slot tile")
	}
	tile := f.tiles[fitted/fitMaxCells][fitted%fitMaxCells]
	if got := tile.btn.GetWidget().ToolTips; len(got) != 1 {
		t.Fatalf("fitted slot tile carries %d tooltips, want 1", len(got))
	}
	m := tile.tip.View.Model()
	if m.Title != "Small Autocannon" || m.Sub != "High slot 1 · Weapon" {
		t.Errorf("slot tile tooltip = %q/%q, want Small Autocannon / High slot 1 · Weapon", m.Title, m.Sub)
	}
	if def, ok := env.Content.Module("mod-autocannon-s"); !ok || m.Icon != theme.ModuleIconFor(def) {
		t.Errorf("slot tile icon = %q, want the module's own icon (mod_autocannon_s)", m.Icon)
	}
	if len(m.Hints) != 0 {
		t.Errorf("slot tile carries rack hints %q — a fitting tile neither clicks nor cycles ammo", m.Hints)
	}
	if tipSection(m, "CHARGE") != nil {
		t.Error("slot tile guesses a charge it cannot know")
	}

	// An empty slot tile has no module to describe.
	if empty >= 0 {
		tile := f.tiles[empty/fitMaxCells][empty%fitMaxCells]
		if got := tile.btn.GetWidget().ToolTips; len(got) != 0 {
			t.Errorf("empty slot tile carries %d tooltips, want 0", len(got))
		}
	}

	// Hangar: fit-able modules get the slotless panel, stored hulls none.
	env.Sim.Hangar.Items = append(env.Sim.Hangar.Items,
		content.CargoStack{Item: "mod-artillery-s", Qty: 1})
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships, sim.StoredShip{Hull: "ship-sloop"})
	frame()
	if len(f.hangar.Rows) == 0 {
		t.Fatal("hangar table is empty after seeding it")
	}
	for i, r := range f.hangar.Rows {
		w := f.hangar.RowWidget(i)
		if w == nil {
			continue
		}
		switch data := r.Data.(type) {
		case content.ModuleID:
			if len(w.GetWidget().ToolTips) != 1 {
				t.Fatalf("hangar module row %d carries %d tooltips, want 1", i, len(w.GetWidget().ToolTips))
			}
			def, ok := env.Content.Module(data)
			if !ok {
				t.Fatalf("hangar module %s missing from content", data)
			}
			hm := f.hangar.Tips[i].View.Model()
			if hm.Title != def.Name || hm.Sub != widgets.TipSub("", def.Kind) {
				t.Errorf("hangar module tooltip = %q/%q, want %q/%q",
					hm.Title, hm.Sub, def.Name, widgets.TipSub("", def.Kind))
			}
			// No slot, no charge, no rack hints: a hangar row is neither.
			if len(hm.Hints) != 0 {
				t.Errorf("hangar module tooltip carries rack hints %q", hm.Hints)
			}
		case content.ShipID:
			if len(w.GetWidget().ToolTips) != 0 {
				t.Errorf("hangar hull row %d carries a module tooltip", i)
			}
		}
	}
}

// TestFittingSlotGrid pins the grid to the hull: one tile per hull slot
// (empty ones dashed, so the grid shows the hull's shape), filled tiles
// carrying the module glyph centred in the 50×40 cell.
func TestFittingSlotGrid(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)

	hull, ok := env.Content.Ship("ship-sloop")
	if !ok {
		t.Fatal("fixture: ship-sloop missing")
	}
	counts := []int{hull.Slots.High, hull.Slots.Mid, hull.Slots.Low}
	fit, _ := env.Sim.Fittings.Get(env.Sim.Player())
	fitted := []int{len(fit.High), len(fit.Mid), len(fit.Low)}
	for row := 0; row < 3; row++ {
		shown := 0
		for idx, tile := range f.tiles[row] {
			if !fitTileShown(tile) {
				continue
			}
			shown++
			if want := tile.mod != ""; want != (idx < fitted[row]) {
				t.Errorf("%s tile %d: filled=%v with %d fitted modules", fitClasses[row], idx, want, fitted[row])
			}
			if idx >= fitted[row] && !tile.empty {
				t.Errorf("%s tile %d holds no module but is not the dashed placeholder", fitClasses[row], idx)
			}
			if idx >= counts[row] {
				t.Errorf("%s tile %d shown past the hull's %d slots", fitClasses[row], idx, counts[row])
			}
		}
		if shown != max(counts[row], fitted[row]) {
			t.Errorf("%s row shows %d tiles, want the hull's %d", fitClasses[row], shown, counts[row])
		}
	}

	// Filled tile: the module's own icon (its art) is the button's graphic
	// (the padding pair is the centring condition; the capture test pins the
	// pixels).
	tile := fitTileAt(f, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	def, ok := env.Content.Module("mod-autocannon-s")
	if !ok {
		t.Fatal("fixture: mod-autocannon-s missing from content")
	}
	if tile.icon != theme.ModuleIconFor(def) {
		t.Fatalf("high slot 1 icon = %q, want %s", tile.icon, theme.ModuleIconFor(def))
	}
	if g := tile.btn.GraphicImage(); g == nil || g.Idle == nil || g.Idle == fitGlyph("", color.RGBA{}) {
		t.Error("fitted tile carries no icon")
	}
	if b := tile.btn.GraphicImage().Idle.Bounds(); b.Dx() != theme.Px(theme.IconTile) || b.Dy() != theme.Px(theme.IconTile) {
		t.Errorf("tile icon box = %v, want the %d px tile icon", b, theme.Px(theme.IconTile))
	}
	// Free the last high slot: the placeholder tile takes the dashed
	// chrome instead of a fitted module's accent edge.
	sfit, _ := env.Sim.Fittings.Get(env.Sim.Player())
	sfit.High = sfit.High[:len(sfit.High)-1]
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	empty := fitTileAt(f, sim.SlotRef{Class: content.SlotHigh, Index: len(sfit.High)})
	if !fitTileShown(empty) || !empty.empty || empty.mod != "" {
		t.Fatalf("high slot %d is not the empty placeholder tile", len(sfit.High)+1)
	}
	if empty.btn.Image().Idle == tile.btn.Image().Idle {
		t.Error("empty tile draws the fitted tile's chrome, want the dashed placeholder")
	}
}

// TestFitDashTile: the placeholder raster is the tile's box (the pixels —
// dashes, gaps, hollow centre — are pinned by the capture frame, which is
// the only place ebiten allows a readback).
func TestFitDashTile(t *testing.T) {
	p := theme.DefaultPalette()
	if b := fitDashImage(p).Bounds(); b.Dx() != theme.Px(fitTileW) || b.Dy() != theme.Px(fitTileH) {
		t.Errorf("dash tile raster = %v, want the %d×%d tile box", b, theme.Px(fitTileW), theme.Px(fitTileH))
	}
}

// TestFittingHangarDoubleClickFits: a hangar module's double-click fits it
// into the first empty slot of its class, and a full class is a no-op.
func TestFittingHangarDoubleClickFits(t *testing.T) {
	env, cmds := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }

	env.Sim.Hangar.Items = append(env.Sim.Hangar.Items,
		content.CargoStack{Item: "mod-artillery-s", Qty: 1})
	frame()
	modRow := -1
	for i, r := range f.hangar.Rows {
		if data, ok := r.Data.(content.ModuleID); ok && data == "mod-artillery-s" {
			modRow = i
		}
	}
	if modRow < 0 {
		t.Fatal("fixture: hangar module row missing")
	}

	// Every high slot is fitted: the double-click is a no-op.
	clickRow(ui, f.hangar, modRow)
	clickRow(ui, f.hangar, modRow)
	if len(*cmds) != 0 {
		t.Fatalf("double-click on a full class enqueued %d commands, want 0", len(*cmds))
	}
	if env.InfoWindow().IsOpen() {
		t.Error("hangar double-click still opened Show Info — the gesture is the Fit verb")
	}

	// Free the last high slot: the same gesture fills the first empty one.
	fit, _ := env.Sim.Fittings.Get(env.Sim.Player())
	fit.High = fit.High[:len(fit.High)-1]
	frame()
	clickRow(ui, f.hangar, modRow)
	clickRow(ui, f.hangar, modRow)
	if len(*cmds) != 1 {
		t.Fatalf("double-click enqueued %d commands, want 1", len(*cmds))
	}
	got, ok := (*cmds)[0].(sim.CmdFitModule)
	if !ok {
		t.Fatalf("cmd = %#v, want sim.CmdFitModule", (*cmds)[0])
	}
	want := sim.SlotRef{Class: content.SlotHigh, Index: len(fit.High)}
	if got.Item != "mod-artillery-s" || got.Slot != want {
		t.Errorf("cmd = %#v, want mod-artillery-s into %+v", got, want)
	}
	if env.InfoWindow().IsOpen() {
		t.Error("the fit double-click also opened Show Info")
	}

	// A stored hull keeps the Show Info gesture (its row menu still has
	// the entry too).
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships, sim.StoredShip{Hull: "ship-sloop"})
	frame()
	shipRow := -1
	for i, r := range f.hangar.Rows {
		if _, isShip := r.Data.(content.ShipID); isShip {
			shipRow = i
		}
	}
	if shipRow < 0 {
		t.Fatal("fixture: hangar hull row missing")
	}
	clickRow(ui, f.hangar, shipRow)
	clickRow(ui, f.hangar, shipRow)
	if !env.InfoWindow().IsOpen() {
		t.Error("double-click on a stored hull opened no Show Info")
	}
}

// TestFittingTileDoubleClickShowsInfo: a filled slot tile keeps the Show
// Info double-click; its right-click menu still offers Fit/Unefit.
func TestFittingTileDoubleClickShowsInfo(t *testing.T) {
	env, cmds := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)

	tile := fitTileAt(f, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	clickFitTile(ui, tile)
	clickFitTile(ui, tile)
	f.Sync()
	if !env.InfoWindow().IsOpen() {
		t.Fatal("double-click on a filled tile did not open Show Info")
	}
	if len(*cmds) != 0 {
		t.Errorf("tile clicks enqueued %d commands, want 0", len(*cmds))
	}

	rightClickFitTile(t, ui, tile)
	if e := entry(t, env, "Unefit slot"); e.Disabled {
		t.Error("Unefit disabled for a filled tile")
	}
	if e := entry(t, env, "Fit module"); !e.Disabled {
		t.Error("Fit enabled for a filled tile")
	}
	pressMenuEntry(t, env, "Unefit slot")
	if len(*cmds) != 1 {
		t.Fatalf("Unefit enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdUnfitModule); !ok || c.Slot != tile.slot {
		t.Errorf("cmd = %#v, want CmdUnfitModule{%+v}", (*cmds)[0], tile.slot)
	}
}

// TestFitEHP pins the per-type effective HP: the three layer maxima over
// the penetrating share each resistance leaves.
func TestFitEHP(t *testing.T) {
	ship := sim.ShipState{
		MaxShield: 500, MaxArmor: 400, MaxStructure: 300,
		Resists: content.Resists{EM: 0.5, Thermal: 0.25, Kinetic: 0, Explosive: 0.75},
	}
	for _, tc := range []struct {
		typ  content.DamageType
		want float64
	}{
		{content.DmgEM, 1200 / 0.5},
		{content.DmgThermal, 1200 / 0.75},
		{content.DmgKinetic, 1200},
		{content.DmgExplosive, 1200 / 0.25},
	} {
		if got := fitEHP(ship, tc.typ); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("EHP %s = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

// TestFittingSummaryFigures pins the summary's display math against the
// sim: the predicted DPS follows the shell the gun would load (preference
// first, then cargo order), and the capacitor figures mirror the sim's own
// recharge curve.
func TestFittingSummaryFigures(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()

	// The showcase fit: a small autocannon (small shells, 3 s shot time)
	// and a rocket pod (no ammo, 2.5 s). Cargo order picks the EMP shell
	// first: (18+4+2+1)/3 + (0+5+4+33)/2.5.
	fit, ok := env.Sim.Fittings.Get(env.Sim.Player())
	if !ok {
		t.Fatal("fixture: player has no fitting")
	}
	want := 25.0/3 + 42.0/2.5
	if got := f.totalDPS(*fit, true); math.Abs(got-want) > 1e-9 {
		t.Errorf("DPS = %v, want %v", got, want)
	}
	if got := f.sum[0].value.Label; got != theme.GroupFloat(want, 1) {
		t.Errorf("DPS row = %q, want %q", got, theme.GroupFloat(want, 1))
	}
	if got := env.PredictedAmmo(sim.SlotRef{Class: content.SlotHigh, Index: 0}); got != "ammo-emp-s" {
		t.Fatalf("predicted shell = %q, want ammo-emp-s (cargo order)", got)
	}

	// A preferred charge changes both the prediction and the figure.
	env.Sim.Queue(sim.CmdSetAmmo{Slot: sim.SlotRef{Class: content.SlotHigh, Index: 0}, Item: "ammo-ap-s"})
	env.Sim.Step()
	want = 26.0/3 + 42.0/2.5
	if got := f.totalDPS(*fit, true); math.Abs(got-want) > 1e-9 {
		t.Errorf("DPS with a preferred shell = %v, want %v", got, want)
	}
	if got := env.PredictedAmmo(sim.SlotRef{Class: content.SlotHigh, Index: 0}); got != "ammo-ap-s" {
		t.Fatalf("predicted shell = %q, want the preferred ammo-ap-s", got)
	}

	// Capacitor: no running modules reads stable, the bar empty.
	frame()
	if got := f.verdict.Label; got != "Cap stable" {
		t.Errorf("verdict = %q, want Cap stable", got)
	}
	if f.capFrac != 0 {
		t.Errorf("cap bar fraction = %v, want the empty track", f.capFrac)
	}

	// Every module running: the drain is the sum of cost/cycle, over the
	// curve's peak (the showcase sloop is unstable under a full rack).
	fit.HighRT[1].Active = true // mining laser: 20 GJ / 30 s
	fit.HighRT[2].Active = true // rocket pod: 1 GJ / 2.5 s
	fit.MidRT[0].Active = true  // afterburner: 20 GJ / 10 s
	fit.MidRT[1].Active = true  // shield booster: 30 GJ / 4 s
	fit.MidRT[2].Active = true  // stasis web: 10 GJ / 5 s
	fit.LowRT[0].Active = true  // armor repairer: 25 GJ / 5 s
	wantDrain := 20.0/30 + 1.0/2.5 + 20.0/10 + 30.0/4 + 10.0/5 + 25.0/5
	if got := f.capDrain(*fit, true); math.Abs(got-wantDrain) > 1e-9 {
		t.Errorf("cap drain = %v, want %v", got, wantDrain)
	}
	cap, _ := env.Sim.Caps.Get(env.Sim.Player())
	frame()
	if got, want := f.sum[1].value.Label, theme.GroupFloat(wantDrain, 1)+" GJ/s"; got != want {
		t.Errorf("drain row = %q, want %q", got, want)
	}
	if got, want := f.sum[2].value.Label, theme.GroupFloat(capRechargePeak(*cap), 1)+" GJ/s"; got != want {
		t.Errorf("recharge row = %q, want %q", got, want)
	}
	if !strings.HasPrefix(f.verdict.Label, "Cap unstable (~") {
		t.Errorf("verdict = %q, want the unstable string", f.verdict.Label)
	}
	if f.capFrac != 1 {
		t.Errorf("cap bar fraction = %v, want the capped full bar", f.capFrac)
	}
}

// TestFittingCapRechargeMirrorsSim: the window's peak-recharge figure is
// the sim's own curve — at 25 % capacity the sim recharges exactly
// capRechargePeak(cap)·DT in one tick.
func TestFittingCapRechargeMirrorsSim(t *testing.T) {
	env, _ := newChromeEnv(t)
	player := env.Sim.Player()
	c, ok := env.Sim.Caps.Get(player)
	if !ok || c.Capacity <= 0 || c.Tau <= 0 {
		t.Fatal("fixture: player has no capacitor")
	}
	c.Current = 0.25 * c.Capacity
	before := c.Current
	peak := capRechargePeak(*c)
	if peak <= 0 {
		t.Fatal("peak recharge reads zero")
	}
	env.Sim.Step()
	got := (c.Current - before) / sim.DT
	if math.Abs(got-peak) > 1e-6*peak {
		t.Errorf("sim recharged %v GJ/s at 25%% capacity, window says %v", got, peak)
	}

	// And the endurance mirror: a drain under the peak never empties, a
	// drain over it does.
	if secs := capEndure(*c, peak*0.9); secs != fitCapHorizon {
		t.Errorf("capEndure under the peak = %v s, want the horizon", secs)
	}
	if secs := capEndure(*c, peak*1.5); secs <= 0 || secs >= fitCapHorizon {
		t.Errorf("capEndure over the peak = %v s, want a finite run-out", secs)
	}
	if v := capVerdict(*c, true, peak*0.5, peak); v.text != "Cap stable" || !v.stable {
		t.Errorf("verdict under the peak = %+v, want Cap stable", v)
	}
	if v := capVerdict(*c, true, peak*1.5, peak); v.stable || !strings.HasPrefix(v.text, "Cap unstable (~") {
		t.Errorf("verdict over the peak = %+v, want Cap unstable", v)
	}
}

// TestFittingIdentityBlock: the portrait carries the fitted hull's art and
// the identity line carries the maker's emblem.
func TestFittingIdentityBlock(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)

	shell, ok := env.Content.Ship("ship-sloop")
	if !ok {
		t.Fatal("fixture: ship-sloop missing")
	}
	if f.portraitHull != "ship-sloop" {
		t.Errorf("portrait hull = %q, want ship-sloop", f.portraitHull)
	}
	if f.portraitDraws != 1 {
		t.Errorf("portrait painted %d times for one hull, want the hull-change repaint only", f.portraitDraws)
	}
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	if f.portraitDraws != 1 {
		t.Errorf("portrait repainted on an unchanged hull (%d draws)", f.portraitDraws)
	}
	if f.name.Label != shell.Name {
		t.Errorf("identity name = %q, want %q", f.name.Label, shell.Name)
	}
	if f.emblemSlug != shell.Maker {
		t.Errorf("emblem slug = %q, want %q", f.emblemSlug, shell.Maker)
	}
	if f.idText.Label != makerSub(shell.Class, shell.Maker) {
		t.Errorf("identity line = %q, want %q", f.idText.Label, makerSub(shell.Class, shell.Maker))
	}
	if got := len(f.idRow.Children()); got != 2 {
		t.Errorf("identity row children = %d, want the emblem + the maker line", got)
	}

	// No ship: an empty frame and no emblem (never a stale hull's art).
	env.Sim.Ships.Remove(env.Sim.Player())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	if f.portraitDraws != 2 {
		t.Errorf("portrait painted %d times, want one more on the hull change", f.portraitDraws)
	}
	if len(f.idRow.Children()) != 1 {
		t.Errorf("no-ship identity row keeps %d children, want the bare line", len(f.idRow.Children()))
	}
}

// TestFittingMakerLogoIsCorpArt pins the identity row's maker mark: the
// dropped corporation art (theme.CorpIconFor) at the corp token, not the
// vector emblem — the corp icons replace the emblems on the fitting maker
// block too.
func TestFittingMakerLogoIsCorpArt(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)

	shell, ok := env.Content.Ship("ship-sloop")
	if !ok {
		t.Fatal("fixture: ship-sloop missing")
	}
	kids := f.idRow.Children()
	if len(kids) != 2 {
		t.Fatalf("identity row children = %d, want the maker mark + the line", len(kids))
	}
	g, ok := kids[0].(*widget.Graphic)
	if !ok || g.Image == nil {
		t.Fatalf("maker mark is %T, want a graphic with the corp art", kids[0])
	}
	wantID := theme.CorpIconFor(shell.Maker)
	if wantID == "" {
		t.Fatalf("%s has no drop-in corp art", shell.Maker)
	}
	if b := g.Image.Bounds(); b.Dx() != theme.Px(theme.IconCorp) {
		t.Errorf("maker mark box = %v, want %d px", b, theme.Px(theme.IconCorp))
	}
	if g.Image != icons.Get(wantID, theme.Px(theme.IconCorp), env.Pal().Text) {
		t.Errorf("maker mark is not the drop-in corp art %q", wantID)
	}
}

// TestFittingSeamKept: the hangar seam is the window's one sidecar seam.
func TestFittingSeamKept(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	seams := f.Splits()
	if len(seams) != 1 || seams[0].Name != "fitting-hangar" || seams[0].Index != 0 {
		t.Fatalf("Splits() = %+v, want the fitting-hangar seam", seams)
	}
	if got := seams[0].Split.Ratio(seams[0].Index); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("default seam ratio = %v, want the equal share 0.5", got)
	}
}
