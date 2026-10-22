package widgets

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestTablePlateBand pins the opt-in plate-through viewport's wiring: the
// fill's offset is measured from the band against the laid-out viewport, the
// table without the knob stays at today's offset 0, and unloading the plate
// (an empty band) puts the fill back.
func TestTablePlateBand(t *testing.T) {
	env, _ := newTestEnv(t)
	cols := []Column{{Key: "A", Title: "A"}, {Key: "B", Title: "B"}}
	rows := make([]TableRow, 24)
	for i := range rows {
		rows[i] = TableRow{Cells: []string{fmt.Sprintf("row %d", i), ""}, Data: i}
	}

	newTable := func(opts ...TableOption) (*Table, *ebitenui.UI) {
		tbl := NewTable(env, cols, opts...)
		root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)))
		root.AddChild(tbl.Container())
		ui := &ebitenui.UI{Container: root, PrimaryTheme: env.WidgetTheme()}
		tbl.SetRows(rows)
		return tbl, ui
	}
	frame := func(tbl *Table, ui *ebitenui.UI) {
		tbl.Sync()
		ui.Update()
		ui.Draw(ebiten.NewImage(640, 400))
	}

	plain, plainUI := newTable()
	// The band is the viewport's top 82 px (two rows of the test face).
	band := image.Rect(0, 0, 640, 0)
	var banded *Table
	banded, bandUI := newTable(PlateBand(func() image.Rectangle { return band }))
	band = image.Rect(0, 0, 640, 82)

	frame(plain, plainUI)
	frame(banded, bandUI) // first layout: the offset is measured from it
	frame(plain, plainUI)
	frame(banded, bandUI)

	view := banded.ScrollForTest().ViewRect()
	if view != plain.ScrollForTest().ViewRect() {
		t.Fatalf("viewports differ: %v vs %v (the knob must not move layout)", view, plain.ScrollForTest().ViewRect())
	}
	if want := plateFillTop(band, banded.ScrollForTest().GetWidget().Rect); banded.plateTop != want || want <= 0 {
		t.Errorf("banded fill offset = %d, want %d (the band's bottom edge)", banded.plateTop, want)
	}
	if plain.plateTop != 0 {
		t.Errorf("plain fill offset = %d, want 0 (a table without the knob paints as today)", plain.plateTop)
	}

	// No plate loaded: the knob reports no band and the fill returns.
	band = image.Rectangle{}
	frame(banded, bandUI)
	if banded.plateTop != 0 {
		t.Errorf("fill offset with no band = %d, want 0 (today's viewport)", banded.plateTop)
	}
}

// TestPlateFillTop pins the clamping: a band above the viewport shows art
// through all of it, a band below leaves today's fill, and the empty band
// (no plate loaded) leaves it too.
func TestPlateFillTop(t *testing.T) {
	view := image.Rect(10, 50, 210, 250) // 200 px tall
	for _, c := range []struct {
		name string
		band image.Rectangle
		want int
	}{
		{"empty", image.Rectangle{}, 0},
		{"below", image.Rect(10, 260, 210, 320), 0},
		{"band edge inside", image.Rect(10, 0, 210, 118), 68},
		{"band edge at the top", image.Rect(10, 0, 210, 50), 0},
		{"whole viewport", image.Rect(10, 0, 210, 400), 200},
		{"empty viewport", image.Rect(10, 50, 210, 50), 0},
	} {
		if got := plateFillTop(c.band, view); got != c.want {
			t.Errorf("%s: plateFillTop = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestBandedNineSlice pins the banded viewport's shape: the Bordered border
// ring with the band's height above the fill line. The pixels are pinned by
// the header capture (ebiten readback needs a running game loop).
func TestBandedNineSlice(t *testing.T) {
	body := color.NRGBA{R: 10, G: 20, B: 30, A: 230}
	edge := color.NRGBA{R: 40, G: 50, B: 60, A: 160}
	for _, c := range []struct{ top, want int }{
		{8, 8}, {2, 2}, {1, 2}, {0, 2}, // a band thinner than the border keeps the border
	} {
		mw, mh := banded(body, edge, 2, c.top).MinSize()
		if mw != 4 || mh != c.want+2 {
			t.Errorf("banded(top %d) min size = %d×%d, want 4×%d", c.top, mw, mh, c.want+2)
		}
	}
}
