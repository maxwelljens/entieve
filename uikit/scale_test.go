package uikit

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The scale funnel's contract and the regressions that ride on it:
// every metric goes through Px/Scale (identity at 1), the cursor-facing
// hit tests (grip bands, menu rows) measure in the same scaled units
// they lay out in, and the two stepper/table defects stay fixed at every
// scale. Every test restores scale 1 on cleanup — the baseline suite
// runs at scale 1, where design units are pixels.

// useScale installs s for the test's duration and restores the identity.
func useScale(t *testing.T, s float64) {
	t.Helper()
	SetScale(s)
	t.Cleanup(func() { SetScale(1) })
}

// TestPxFunnel pins the funnel itself: identity at 1 (the baseline), the
// straight multiple at integers and halves, and the broken-value fallback.
func TestPxFunnel(t *testing.T) {
	useScale(t, 1)
	for _, n := range []int{0, 1, 5, 8, 44, 320, 1024} {
		if got := Px(n); got != n {
			t.Errorf("Px(%d) at scale 1 = %d, want %d (identity)", n, got, n)
		}
	}
	SetScale(2)
	if Px(10) != 20 || Px(11) != 22 || Px(320) != 640 {
		t.Errorf("Px at 2 = %d/%d/%d, want 20/22/640", Px(10), Px(11), Px(320))
	}
	SetScale(1.5)
	if Px(8) != 12 || Px(5) != 8 { // round-half-away: 7.5 → 8
		t.Errorf("Px at 1.5 = %d/%d, want 12/8", Px(8), Px(5))
	}
	if Scale() != 1.5 {
		t.Errorf("Scale() = %v, want 1.5", Scale())
	}
	// Broken settings must never collapse the interface to zero.
	SetScale(0)
	if Scale() != 1 || Px(9) != 9 {
		t.Errorf("SetScale(0): scale = %v Px(9) = %d, want 1/9", Scale(), Px(9))
	}
	SetScale(math.NaN())
	if Scale() != 1 {
		t.Errorf("SetScale(NaN) = %v, want 1", Scale())
	}
	SetScale(1)
}

// TestScaleDoublesConstructionMetrics: at 2× every construction-time
// metric doubles — window MinSize, the fallback Location, the title-bar
// height, button hit boxes, the sort-header strip — and the type scale
// doubles with it (faces are built from the scaled TypeScale).
func TestScaleDoublesConstructionMetrics(t *testing.T) {
	// Measure the design-size face and a design-size panel first (scale
	// 1), then their 2× twins.
	env1, _ := newTestEnv(t)
	w1, _ := text.Measure("Entieve", *env1.Face(StepBase), 0)
	inv1 := NewInventory(env1)
	// Pump once: the buttons carry no images of their own and validate
	// against the window's theme (an unthemed PreferredSize panics).
	pumpWindow(t, newWindowUI(t, env1, inv1.Win()), nil, 1280, 720)
	bar1W, _ := inv1.qtyBar.PreferredSize()

	useScale(t, 2)
	env, _ := newTestEnv(t)

	if env.TS.Base != 42 || env.TS.Small != 33.6 {
		t.Errorf("env TS at 2 = %v, want Base 42 / Small 33.6 (design 21/16.8)", env.TS)
	}
	w2, _ := text.Measure("Entieve", *env.Face(StepBase), 0)
	// Glyph advances carry a sub-1/1000 px quantization, so "double"
	// holds to well under a tenth of a pixel, not bit-exactly.
	if math.Abs(w2-2*w1) > 0.05 {
		t.Errorf("measure at 2× = %v, want ≈%v (double within glyph quantization)", w2, 2*w1)
	}

	inv := NewInventory(env)
	if inv.win.MinSize == nil || inv.win.MinSize.X != 600 || inv.win.MinSize.Y != 840 {
		t.Fatalf("inventory MinSize at 2 = %v, want (600,840) (2× the design 300×420)", inv.win.MinSize)
	}
	if loc := inv.Win().GetContainer().GetWidget().Rect; loc != image.Rect(Px(396), Px(256), Px(701), Px(690)) {
		t.Errorf("fallback Location at 2 = %v, want %v", loc, image.Rect(Px(396), Px(256), Px(701), Px(690)))
	}
	if w := inv.plus.GetWidget().MinWidth; w != 80 || inv.plus.GetWidget().MinHeight != 80 {
		t.Errorf("stepper + MinSize at 2 = %d×%d, want 80×80 (40 design hit box)", w, inv.plus.GetWidget().MinHeight)
	}

	// Title bar: pump the window — the title child is exactly Px(44).
	ui := newWindowUI(t, env, inv.Win())
	pumpWindow(t, ui, nil, 1280, 720)
	kids := inv.Win().GetContainer().Children()
	h := -1
	if len(kids) > 0 {
		h = kids[0].GetWidget().Rect.Dy()
	}
	if h != Px(titleBarH) {
		t.Errorf("title bar height at 2 = %d, want %d (Px(44))", h, Px(titleBarH))
	}

	// Sort header strip: the table's header buttons carry MinSize(0, 40).
	tbl := NewTable(env, []Column{{Key: "A", Title: "A", Priority: 0}})
	if h := tbl.hdrBtn[0].GetWidget().MinHeight; h != Px(tableHeaderH) || h != 80 {
		t.Errorf("header MinHeight at 2 = %d, want 80 (Px(design 40))", h)
	}

	// PreferredSize doubles: the stepper row's preferred width is all
	// scaled pieces (40 px floors, gaps, measured labels). Glyph
	// advances quantize per size, so "2×" holds to a couple of percent.
	bar2W, _ := inv.qtyBar.PreferredSize()
	if math.Abs(float64(bar2W)-2*float64(bar1W)) > 0.05*float64(bar2W) {
		t.Errorf("stepper row preferred width 1×=%d 2×=%d, want ≈2× (±5%%)", bar1W, bar2W)
	}
}

// TestGripBandScalesWithUI: the resize grip measures its band with
// Px(edgeGripPx) in the same units the cursor arrives in, so at 1.5× a
// press 10 px inside the left edge still grips and one exactly 12 px in
// does not (the band grew with everything else).
func TestGripBandScalesWithUI(t *testing.T) {
	useScale(t, 1.5)
	if Px(edgeGripPx) != 12 {
		t.Fatalf("Px(edgeGripPx) at 1.5 = %d, want 12", Px(edgeGripPx))
	}
	win, ch := newChromeTestWindow(t)
	rect := win.GetContainer().GetWidget().Rect

	inside := image.Point{X: rect.Min.X + 10, Y: rect.Min.Y + 50}
	if e := ch.gripStart(inside); e&edgeLeft == 0 {
		t.Errorf("press %v (10 px in, band %d): edges = %b, want edgeLeft", inside, Px(edgeGripPx), e)
	}
	outside := image.Point{X: rect.Min.X + Px(edgeGripPx), Y: rect.Min.Y + 50}
	if e := ch.gripStart(outside); e != 0 {
		t.Errorf("press %v (exactly band width in): edges = %b, want 0 (band is %d)",
			outside, e, Px(edgeGripPx))
	}
}

// TestMenuHitTestMatchesScaledLayout: the popup lays out its row bands
// and label offsets from the same scaled constants, so at 1.5× the point
// the label draws at hits its own row and a point above the popup hits
// nothing — hit test and paint agree in Layout units at every scale.
func TestMenuHitTestMatchesScaledLayout(t *testing.T) {
	useScale(t, 1.5)
	env, _ := newTestEnv(t)
	m := env.Menu()
	if m == nil {
		t.Fatal("env has no menu")
	}
	at := image.Point{X: 400, Y: 300}
	m.Open([]MenuEntry{{Label: "Show Info", Bold: true}, MenuSep, {Label: "Warp"}}, at)
	defer m.Close()

	pop := m.Rect()
	if pop.Dx() < Px(menuMinW) {
		t.Errorf("popup width %d, want ≥ scaled menuMinW %d", pop.Dx(), Px(menuMinW))
	}
	row0 := m.RowRect(0)
	if want := tipLineH(env.FaceBold(StepSmall)) + 2*Px(menuRowPadY); row0.Dy() != want {
		t.Errorf("row0 height %d, want the scaled bold line box + 2×%d = %d",
			row0.Dy(), Px(menuRowPadY), want)
	}
	// The label draw anchor lands inside its own row band...
	labelPt := image.Point{X: row0.Min.X + Px(menuRowPadX), Y: row0.Min.Y + Px(menuRowPadY)}
	if i, ok := m.at(labelPt); !ok || i != 0 {
		t.Errorf("hit at the row-0 label anchor %v = (%d,%v), want (0,true)", labelPt, i, ok)
	}
	// ...and one pixel above the popup hits nothing (scaled boundary).
	if _, ok := m.at(image.Point{X: labelPt.X, Y: pop.Min.Y - 1}); ok {
		t.Errorf("hit one px above the popup (y=%d) succeeded, want none", pop.Min.Y-1)
	}
	// The last band still ends exactly one scaled bottom pad above the popup.
	last := m.RowRect(len(m.Entries()) - 1)
	if last.Max.Y != pop.Max.Y-Px(menuPadY) {
		t.Errorf("last row ends %d, popup ends %d (bottom pad %d)",
			last.Max.Y, pop.Max.Y, Px(menuPadY))
	}
}

// TestHeaderCellAnchorAgreement (Defect A regression): every header
// label sits at the SAME x its column's cell text uses — left anchors at
// the column's left edge, right anchors at the column's right edge minus
// cellPad — at scale 1 and at 1.5, for both alignment sides. The
// right-cell anchor accounts for ebitenui's End-positioning math
// (glyphs land Padding.Left+2×Padding.Right left of the rect edge;
// Padding.Right is 0 on purpose — see the Defect A note in newRow).
func TestHeaderCellAnchorAgreement(t *testing.T) {
	defer SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		SetScale(sc)
		env, _ := newTestEnv(t)
		cols := []Column{
			{Key: "Item", Title: "Item", Priority: 0},
			{Key: "Qty", Title: "Qty", Priority: 1, AlignRight: true},
			{Key: "Vol", Title: "m³", Priority: 2, AlignRight: true},
		}
		tbl, ui := newPumpedTable(t, env, cols, ContentsFace(env.FaceMono(StepSmall)))
		tbl.SetRows([]TableRow{{Cells: []string{"Tritanium", "×1,000", "100.0"}, Data: 0}})
		// SetRows refits; the cells get their rects on the next frame.
		ui.Update()
		ui.Draw(ebiten.NewImage(640, 400))

		for i, c := range cols {
			// Header label glyph box: the button's anchor padding put it
			// there and its own text padding is zero.
			lbl := tbl.hdrBtn[i].Text().GetWidget().Rect
			cell := tbl.texts[0][i].GetWidget().Rect
			if c.AlignRight {
				cellEnd := cell.Max.X - Px(cellPad)
				if lbl.Max.X != cellEnd {
					t.Errorf("scale %v col %q: header label ends %d, cell text ends %d (want equal)",
						sc, c.Title, lbl.Max.X, cellEnd)
				}
			} else {
				if lbl.Min.X != cell.Min.X {
					t.Errorf("scale %v col %q: header label starts %d, cell text starts %d (want equal)",
						sc, c.Title, lbl.Min.X, cell.Min.X)
				}
			}
		}
	}
}

// TestStepperReadoutFitsItsValue (Defect B regression): the qty readout
// resizes to its current value through the row relayout — "Qty 1,000,000"
// owns a box big enough for itself and never reaches the + button, at
// scale 1 and 1.5 — and the stepped path (up to the ladder's 100) stays
// overlap-free too.
func TestStepperReadoutFitsItsValue(t *testing.T) {
	defer SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		SetScale(sc)
		env, _ := newTestEnv(t)
		inv := NewInventory(env)
		ui := newWindowUI(t, env, inv.Win())
		inv.Sync()
		pumpWindow(t, ui, inv.Sync, 1280, 720)

		// doSync runs the panel Sync first (the production frame order);
		// the white-box big-value checks skip it because Sync re-derives
		// the readout from the clamped ladder.
		check := func(want string, doSync bool) {
			t.Helper()
			var sync func()
			if doSync {
				sync = inv.Sync
			}
			pumpWindow(t, ui, sync, 1280, 720)
			lbl := inv.qtyLabel.GetWidget().Rect
			plus := inv.plus.GetWidget().Rect
			minus := inv.minus.GetWidget().Rect
			if inv.qtyLabel.Label != want {
				t.Errorf("scale %v: readout = %q, want %q", sc, inv.qtyLabel.Label, want)
			}
			// The box measures the value itself (same face, no h-padding).
			need, _ := text.Measure(want, *env.FaceMono(StepSmall), 0)
			if float64(lbl.Dx()) < need {
				t.Errorf("scale %v: readout box %d wide, need ≥ %v for %q",
					sc, lbl.Dx(), need, want)
			}
			if lbl.Max.X > plus.Min.X {
				t.Errorf("scale %v: readout (%v) overlaps + button (%v)", sc, lbl, plus)
			}
			if lbl.Min.X < minus.Max.X {
				t.Errorf("scale %v: readout (%v) overlaps − button (%v)", sc, lbl, minus)
			}
		}

		// The stepped path (clamped ladder top), then the wide value the
		// readout must grow for — setStepperValue is exactly what Sync and
		// stepQty call, so this is the production path with a big number.
		inv.qtyIdx = len(qtySteps) - 1
		inv.stepQty(1)
		check("Qty 100", true)
		setStepperValue(inv.qtyBar, inv.qtyLabel, "Qty 1,000,000")
		check("Qty 1,000,000", false)
		inv.qtyIdx = 0
		check("Qty 1", true)
	}
}

// TestSettingsPanelWiring pins the panel's seam: Sync renders the host's
// state (scale value, fullscreen label) and the controls call back with
// the right intents — nothing is stored panel-side, which is what lets a
// live scale change rebuild the window under itself.
func TestSettingsPanelWiring(t *testing.T) {
	env, _ := newTestEnv(t)
	var (
		scale     = 1.0
		fs        = false
		stepDelta []int
		resets    int
	)
	s := NewSettings(env, SettingsActions{
		UIScale:       func() float64 { return scale },
		StepScale:     func(d int) { stepDelta = append(stepDelta, d) },
		Fullscreen:    func() bool { return fs },
		SetFullscreen: func(on bool) { fs = on },
		ResetLayout:   func() { resets++ },
	})
	ui := newWindowUI(t, env, s.Win())
	pumpWindow(t, ui, s.Sync, 1280, 720)

	if got := s.scaleVal.Label; got != "1×" {
		t.Errorf("scale readout = %q, want %q", got, "1×")
	}
	if got := s.fsBtn.Text().Label; got != "Fullscreen: off" {
		t.Errorf("fullscreen label = %q, want off state", got)
	}

	// Buttons: the scale stepper's −/+ are the row's first/last children.
	kids := s.scaleRow.Children()
	if len(kids) != 4 {
		t.Fatalf("scale row children = %d, want 4 (− label value +)", len(kids))
	}
	fireButton(ui, kids[3].(*widget.Button)) // +
	if len(stepDelta) != 1 || stepDelta[0] != 1 {
		t.Errorf("after +: step deltas = %v, want [1]", stepDelta)
	}
	fireButton(ui, kids[0].(*widget.Button)) // −
	if len(stepDelta) != 2 || stepDelta[1] != -1 {
		t.Errorf("after −: step deltas = %v, want [1 -1]", stepDelta)
	}

	// The window's second child is the body: [scale row, fullscreen,
	// reset, hint].
	body, ok := s.win.GetContainer().Children()[1].(widget.Containerer)
	if !ok {
		t.Fatal("settings body is not a container")
	}
	bkids := body.Children()
	if len(bkids) < 4 {
		t.Fatalf("settings body children = %d, want ≥ 4", len(bkids))
	}
	fireButton(ui, bkids[1].(*widget.Button)) // fullscreen toggle (host currently off)
	fs = false
	s.Sync()
	fireButton(ui, bkids[1].(*widget.Button))
	if !fs {
		t.Error("fullscreen button did not flip the host flag")
	}
	s.Sync()
	if got := s.fsBtn.Text().Label; got != "Fullscreen: on" {
		t.Errorf("fullscreen label after click = %q, want on state", got)
	}
	fireButton(ui, bkids[2].(*widget.Button)) // reset layout
	if resets != 1 {
		t.Errorf("reset calls = %d, want 1", resets)
	}
}
