package screens

import (
	"image"
	"image/color"
	"testing"

	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The overview used to emit every visible grid entity unconditionally (sync:
// "for _, info := range k.cv.Grid() { if !info.Visible { continue } ... }")
// and carried no filter and no cog. The filter window adds both, so the
// DEFAULT state is pinned here as exactly the old output — the same rows,
// the same tints — and every switch is pinned to touch the list alone.

// ovRowView is one overview row as the table holds it.
type ovRowView struct {
	entity   ecs.Entity
	kind     string
	tint     color.RGBA
	iconTint color.RGBA
}

// ovRows is the overview's live row set.
func ovRows(k *Kit) []ovRowView {
	var out []ovRowView
	for _, r := range k.ov.tbl.RowsForTest() {
		row, ok := r.Data.(*ovRow)
		if !ok {
			continue
		}
		out = append(out, ovRowView{row.info.Entity, row.info.Kind, r.Tint, r.IconTint})
	}
	return out
}

// ovVisible is how many visible entities the grid offers (the pre-filter
// row count).
func ovVisible(k *Kit) int {
	n := 0
	for _, info := range k.Sim.Grid() {
		if info.Visible {
			n++
		}
	}
	return n
}

// TestOverviewFilterDefaultIsEveryKind: with nothing switched, the rows and
// tints are exactly what the prefix sync produced — every visible entity
// listed, the pilot's hull the only tinted row.
func TestOverviewFilterDefaultIsEveryKind(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1024, 700)

	if n, want := len(ovRows(k)), ovVisible(k); n != want {
		t.Fatalf("default rows = %d, want all %d visible entities", n, want)
	}
	if len(k.OverviewFilter()) != 0 {
		t.Errorf("a fresh kit carries filter state: %v", k.OverviewFilter())
	}
	pal := k.th.Pal()
	for _, r := range ovRows(k) {
		if r.entity == k.Sim.Player() {
			if want := widgets.WithA(pal.Player, 60); r.tint != want {
				t.Errorf("own-ship row tint = %v, want %v", r.tint, want)
			}
			if r.iconTint != pal.Info {
				t.Errorf("own-ship icon tint = %v, want Info %v", r.iconTint, pal.Info)
			}
			continue
		}
		if r.tint != (color.RGBA{}) || r.iconTint != (color.RGBA{}) {
			t.Errorf("%s row carries a tint with every switch on (%v / %v)", r.kind, r.tint, r.iconTint)
		}
	}
}

// TestOverviewFilterHidesKindButNeverThePilot: switching a kind off drops
// its rows at once; KindPlayer has no switch and is never hidden, even by a
// hand-edited sidecar that names it; back to the default lists everything.
func TestOverviewFilterHidesKindButNeverThePilot(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1024, 700)
	stations := 0
	for _, r := range ovRows(k) {
		if r.kind == sim.KindStation {
			stations++
		}
	}
	if stations == 0 {
		t.Fatal("fixture: the showcase grid lists no station to hide")
	}

	k.SetOverviewFilter(OvFilter{sim.KindStation: {Hidden: true}})
	pumpKit(t, k, 1024, 700)
	rows := ovRows(k)
	for _, r := range rows {
		if r.kind == sim.KindStation {
			t.Fatalf("a hidden kind still lists %v", r.entity)
		}
	}
	if !listsEntity(rows, k.Sim.Player()) {
		t.Error("hiding stations also lost the pilot's own row")
	}

	// The pilot's kind is exempt even when a file names it.
	k.SetOverviewFilter(OvFilter{sim.KindPlayer: {Hidden: true}})
	pumpKit(t, k, 1024, 700)
	if !listsEntity(ovRows(k), k.Sim.Player()) {
		t.Fatal("the pilot's own row can be hidden — KindPlayer must be exempt")
	}

	// Defaults restore the whole list.
	k.SetOverviewFilter(nil)
	pumpKit(t, k, 1024, 700)
	if n, want := len(ovRows(k)), ovVisible(k); n != want {
		t.Errorf("restored rows = %d, want %d", n, want)
	}
}

// listsEntity reports whether the row set holds e.
func listsEntity(rows []ovRowView, e ecs.Entity) bool {
	for _, r := range rows {
		if r.entity == e {
			return true
		}
	}
	return false
}

// TestOverviewFilterColourOffZeroesTints: a kind with its colour switch off
// loses BOTH its row tint and its icon tint — plain rows, neutral icons —
// and switching it back restores today's colours. The pilot's row is the
// only tinted one in the fixture; hostile ships take the same branch.
func TestOverviewFilterColourOffZeroesTints(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1024, 700)

	k.SetOverviewFilter(OvFilter{sim.KindPlayer: {Plain: true}})
	pumpKit(t, k, 1024, 700)
	for _, r := range ovRows(k) {
		if r.entity == k.Sim.Player() {
			if r.tint != (color.RGBA{}) || r.iconTint != (color.RGBA{}) {
				t.Fatalf("colour off still tints the pilot's row (%v / %v)", r.tint, r.iconTint)
			}
		}
	}

	k.SetOverviewFilter(nil)
	pumpKit(t, k, 1024, 700)
	pal := k.th.Pal()
	for _, r := range ovRows(k) {
		if r.entity == k.Sim.Player() && r.tint != widgets.WithA(pal.Player, 60) {
			t.Errorf("colour back on did not restore the own-ship tint (%v)", r.tint)
		}
	}
}

// TestOverviewFilterWindowOpensFromTheCog: the cog in the overview's title
// bar (left of the ×) is the window's only door; the window lists every kind
// with both ticks on at the default, and the ticks drive the list.
func TestOverviewFilterWindowOpensFromTheCog(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1024, 700)

	if k.OverviewFilterOpen() {
		t.Fatal("the filter window is up before anything opened it")
	}
	if k.ovCog == nil {
		t.Fatal("the overview title bar carries no cog")
	}
	bar := k.OverviewWindow().TitleBar.GetWidget().Rect
	cog := k.ovCog.GetWidget().Rect
	if !cog.In(bar) {
		t.Errorf("cog %v is outside the title bar %v", cog, bar)
	}
	closeBtn := k.CloseButtonOf(k.OverviewWindow())
	if closeBtn == nil {
		t.Fatal("fixture: the overview has no ×")
	}
	if x := closeBtn.GetWidget().Rect; cog.Overlaps(x) {
		t.Errorf("cog %v overlaps the × %v", cog, x)
	}

	uitest.FireButton(k.ui, k.ovCog)
	pumpKit(t, k, 1024, 700)
	if !k.OverviewFilterOpen() {
		t.Fatal("pressing the cog did not open the filter window")
	}

	f := k.ovFiltWin
	if len(f.rows) != len(ovFilterKinds) {
		t.Fatalf("filter window lists %d kinds, want %d", len(f.rows), len(ovFilterKinds))
	}
	for i, r := range f.rows {
		if r.kind != ovFilterKinds[i] {
			t.Errorf("row %d is %q, want %q", i, r.kind, ovFilterKinds[i])
		}
		if !r.show.Checked() || !r.col.Checked() {
			t.Errorf("%s opens with a switch off", r.kind)
		}
	}

	// The ticker box in the window drives the list: switch the station's
	// Show off through the widget's own input path.
	var station *ovFilterRow
	for i := range f.rows {
		if f.rows[i].kind == sim.KindStation {
			station = &f.rows[i]
		}
	}
	if station == nil {
		t.Fatal("fixture: no station row in the filter window")
	}
	r := station.show.GetWidget().Rect
	station.show.Step(image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2), true)
	pumpKit(t, k, 1024, 700)
	if k.OverviewFilter().Shown(sim.KindStation) {
		t.Error("the Show tick did not reach the overview's filter state")
	}
	for _, row := range ovRows(k) {
		if row.kind == sim.KindStation {
			t.Fatal("the Show tick did not drop the station rows")
		}
	}

	k.CloseOverviewFilter()
	pumpKit(t, k, 1024, 700)
	if k.OverviewFilterOpen() {
		t.Error("CloseOverviewFilter left the window up")
	}
}

// TestOverviewCogCentredAndClearOfClose pins the cog's alignment: the glyph
// box sits centred inside its square button — the glyph must not ride
// ebitenui's graphic slot, whose fixed [label][10 px spacing] offset pinned
// it to the right border (measured before the fix: insets L20 R0 T10 B10) —
// and the cell stops a hairline short of the ×'s strip, whose left edge the
// button's right edge used to land on exactly (gap 0). At scale 1 and 1.5,
// through the Px funnel.
func TestOverviewCogCentredAndClearOfClose(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		uitest.UseScale(t, scale)
		k, _ := newTestKit(t)
		pumpKit(t, k, 1024, 700)
		btn := k.ovCog.GetWidget().Rect
		glyph := k.ovCogGlyph.GetWidget().Rect
		if box := theme.Px(closeBtnH); btn.Dx() != box || btn.Dy() != box {
			t.Errorf("scale %v: cog button = %v, want the %d design square", scale, btn, closeBtnH)
		}
		if box := theme.Px(theme.IconButton); glyph.Dx() != box || glyph.Dy() != box {
			t.Errorf("scale %v: cog glyph = %v, want %d² design", scale, glyph, theme.IconButton)
		}
		l, r := glyph.Min.X-btn.Min.X, btn.Max.X-glyph.Max.X
		tp, bt := glyph.Min.Y-btn.Min.Y, btn.Max.Y-glyph.Max.Y
		if floor := theme.Px(3); l < floor || r < floor || tp < floor || bt < floor {
			t.Errorf("scale %v: glyph margins L%d R%d T%d B%d, want ≥%d px on every side",
				scale, l, r, tp, bt, floor)
		}
		if abs(l-r) > 1 || abs(tp-bt) > 1 {
			t.Errorf("scale %v: glyph margins L%d R%d T%d B%d, want centred ±1", scale, l, r, tp, bt)
		}
		x := k.CloseButtonOf(k.OverviewWindow()).GetWidget().Rect
		if gap := x.Min.X - btn.Max.X; gap < theme.Px(ovCogGap) {
			t.Errorf("scale %v: cog %v touches the × %v — gap %d px, want ≥%d design",
				scale, btn, x, gap, ovCogGap)
		}
	}
}

// TestOverviewFilterWindowFitsContent: the window's floor is the measured
// body plus a cushion, so no kind row can clip at the size it opens at.
func TestOverviewFilterWindowFitsContent(t *testing.T) {
	k, _ := newTestKit(t)
	k.OpenOverviewFilter()
	pumpKit(t, k, 1024, 700)

	win := k.ovFiltWin.win
	if win.TitleBar == nil {
		t.Fatal("the filter window has no title bar")
	}
	rect := win.GetContainer().GetWidget().Rect
	avail := rect.Dy() - win.TitleBar.GetWidget().Rect.Dy()
	_, need := win.Contents.PreferredSize()
	if need > avail {
		t.Errorf("filter body needs %d px, the window gives %d", need, avail)
	}
	for _, r := range k.ovFiltWin.rows {
		for _, cb := range []*widgets.CheckBox{r.show, r.col} {
			if box := cb.GetWidget().Rect; !box.In(rect) {
				t.Fatalf("%s switch %v is outside the window %v", r.kind, box, rect)
			}
		}
	}
}
