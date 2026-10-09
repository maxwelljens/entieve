package ui

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// smokeUI builds a real UI over the embedded content and a live sim.
func smokeUI(t *testing.T) (*UI, *sim.Sim) {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	world := c.Worlds["world-default"]
	if world == nil {
		t.Fatal("world-default missing")
	}
	s, err := sim.New(c, world, 1)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	return New(c, s, func(sim.Command) {}), s
}

// TestDrawSmoke walks every window's draw path at both design sizes and at
// minimum size with overlays open — index/geometry bugs in the derived
// layout surface as panics here (headless: offscreen images only).
func TestDrawSmoke(t *testing.T) {
	for _, size := range designSizes {
		u, s := smokeUI(t)
		u.screenW, u.screenH = size[0], size[1]
		img := ebiten.NewImage(int(size[0]), int(size[1]))
		cam := render.Camera{Zoom: 0.02, ScreenW: size[0], ScreenH: size[1]}

		// Base HUD.
		u.Draw(img, cam, s)
		// All overlays open at once (worst case).
		u.SetWindowOpen(WinInventory, true)
		u.SetWindowOpen(WinFitting, true)
		u.SetWindowOpen(WinScanner, true)
		u.ovFilterOpen = true
		u.Draw(img, cam, s)
		// Station services (all four tabs) drawn directly: the smoke sim
		// starts undocked, so the window would be hidden by the veil.
		reg := u.reg()
		for tab := 0; tab < numStationTabs; tab++ {
			u.stationTab = tab
			u.drawStation(img, s, reg)
		}
		// Starmap + a selected entity path.
		u.DrawStarmap(img)
		u.Selected = s.Player()
		u.Draw(img, cam, s)
	}
}

// TestDrawSmokeScrolledAndHovered: drawing with a scroll offset, hover
// state and a module tooltip pending doesn't escape the panel or panic.
func TestDrawSmokeScrolledAndHovered(t *testing.T) {
	u, s := smokeUI(t)
	u.screenW, u.screenH = 1280, 720
	img := ebiten.NewImage(1280, 720)
	cam := render.Camera{Zoom: 0.02, ScreenW: 1280, ScreenH: 720}

	// Force rows + hover state: step the sim so grid entities exist.
	for i := 0; i < 600; i++ {
		s.Step()
	}
	u.ovScroll = 100 // non-zero offset: partial rows clip to the body
	cell := ModuleCellRect(u.reg().Modules, 0, 0)
	u.mx, u.my = cell.X+5, cell.Y+5
	u.hoverRow, u.hoverCell = 0, 0
	u.hoverSince = 0
	u.now = 10 // tooltip delay elapsed
	u.Draw(img, cam, s)

	// Sort + filter states draw too.
	for col := SortKind; col <= SortVelocity; col++ {
		u.sortCol = col
		u.sortAsc = !u.sortAsc
		u.Draw(img, cam, s)
	}
}

// TestUpdateInteractionSmoke drives the input pipeline: keys, scroll,
// drags, right-click ammo cycling and window toggles against a live sim.
func TestUpdateInteractionSmoke(t *testing.T) {
	u, _ := smokeUI(t)
	// A smaller screen makes the overview scrollable with only the start
	// grid's landmark rows (the property itself is size-independent).
	u.screenW, u.screenH = 900, 520
	reg := u.reg()

	step := func(in Input) {
		t.Helper()
		in.RealSec = 1.0 / 60.0
		if err := u.Update(in); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}

	// Toggle every closable window via its key path.
	step(Input{JustPressed: []ebiten.Key{ebiten.KeyC}})
	if !u.WindowOpen(WinScanner) {
		t.Fatal("C did not open the scanner")
	}
	step(Input{JustPressed: []ebiten.Key{ebiten.KeyI}})
	step(Input{JustPressed: []ebiten.Key{ebiten.KeyF}})

	// Scroll the overview under the cursor.
	ov := reg.Overview
	step(Input{MouseX: ov.X + 20, MouseY: ov.Y + 60, Scroll: -3})
	if u.ovScroll <= 0 {
		t.Fatalf("wheel over overview did not scroll (ovScroll=%v)", u.ovScroll)
	}
	step(Input{MouseX: ov.X + 20, MouseY: ov.Y + 60, Scroll: 100})
	if u.ovScroll != 0 {
		t.Fatalf("scroll not clamped low: %v", u.ovScroll)
	}

	// Sort by clicking the name header.
	step(Input{MouseX: OverviewColumn(ov, 1).X + 4, MouseY: ov.Y + 1 + titleBarH + 4,
		JustClicked: [2]bool{true}})
	if u.sortCol != SortName {
		t.Fatalf("header click did not sort by name: %v", u.sortCol)
	}

	// Drag the overview by its title bar (no buttons under the point).
	tb := TitleBarRect(ov)
	step(Input{MouseX: tb.X + 40, MouseY: tb.Y + 10, JustClicked: [2]bool{true},
		MouseButton: [3]bool{true}})
	if u.drag == nil || u.drag.id != WinOverview {
		t.Fatal("title press did not start a drag")
	}
	step(Input{MouseX: tb.X + 140, MouseY: tb.Y + 110, MouseButton: [3]bool{true}})
	moved := u.WindowRect(WinOverview, u.reg())
	if moved.X == reg.Overview.X && moved.Y == reg.Overview.Y {
		t.Fatal("drag did not move the window")
	}
	if moved.Right() > u.screenW || moved.Bottom() > u.screenH {
		t.Fatalf("dragged window escaped the screen: %v", moved)
	}
	step(Input{MouseX: tb.X + 140, MouseY: tb.Y + 110}) // release
	if u.drag != nil {
		t.Fatal("drag did not end on release")
	}

	// Sort-indicator + filter popup: open the popup and toggle a group.
	step(Input{MouseX: OverviewFilterButtonRect(u.WindowRect(WinOverview, u.reg())).X + 4,
		MouseY:      OverviewFilterButtonRect(u.WindowRect(WinOverview, u.reg())).Y + 4,
		JustClicked: [2]bool{true}})
	if !u.ovFilterOpen {
		t.Fatal("filter button did not open the popup")
	}
	p := OverviewFilterPopupRect(u.WindowRect(WinOverview, u.reg()))
	step(Input{MouseX: p.X + 8, MouseY: OverviewFilterRowRect(p, 0).Y + 8,
		JustClicked: [2]bool{true}})
	if u.ovFilters[FilterShips] {
		t.Fatal("filter row click did not toggle ships off")
	}
	if !u.ovFilters.Passes(sim.KindPlayer) {
		t.Fatal("player must stay visible with ships filtered")
	}
	u.ovFilters = AllFilters()

	// Overview row selection.
	rowY := OverviewRowRect(u.WindowRect(WinOverview, u.reg()), 0, 0).Y + 4
	step(Input{MouseX: u.WindowRect(WinOverview, u.reg()).X + 30, MouseY: rowY,
		JustClicked: [2]bool{true}})

	// Warp-range segments.
	sel := u.WindowRect(WinSelected, u.reg())
	seg := WarpSegRects(sel, len(WarpRanges))[2]
	step(Input{MouseX: seg.X + 5, MouseY: seg.Y + 5, JustClicked: [2]bool{true}})
	if u.warpIdx != 2 {
		t.Fatalf("segment click did not select 50 km: idx=%d", u.warpIdx)
	}

	// Right-click ammo cycling on the first high slot.
	mr := u.WindowRect(WinModules, u.reg())
	cell := ModuleCellRect(mr, 0, 0)
	before := len(u.ammoSel)
	step(Input{MouseX: cell.X + 4, MouseY: cell.Y + 4, JustClicked: [2]bool{false, true}})
	_ = before // may be a no-op depending on fit/cargo; must not panic

	// Close a window through its [×].
	u.SetWindowOpen(WinFitting, true)
	fr := u.WindowRect(WinFitting, u.reg())
	cb := CloseButtonRect(fr)
	step(Input{MouseX: cb.X + 5, MouseY: cb.Y + 5, JustClicked: [2]bool{true}})
	if u.WindowOpen(WinFitting) {
		t.Fatal("[×] did not close the fitting window")
	}
}
