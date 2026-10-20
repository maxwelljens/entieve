package widgets

import (
	"math"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// wheelTable is a 60-row table in its own UI: content overflows the 640×400
// viewport, so the rail is live and the wheel has somewhere to go.
func wheelTable(t *testing.T) (*Table, *ebitenui.UI, *widget.ScrollContainer) {
	t.Helper()
	env, _ := newTestEnv(t)
	tbl, ui := newPumpedTable(t, env, []Column{{Key: "A", Title: "A"}})
	rows := make([]TableRow, 60)
	for i := range rows {
		rows[i] = TableRow{Cells: []string{"row"}, Data: i}
	}
	tbl.SetRows(rows)
	return tbl, ui, tbl.ScrollForTest()
}

// TestScrollRailSettlesAfterFastWheel: two wheel notches on consecutive
// frames (a fast flick), then silence — the offset must never reverse with
// no input, the rail must end up on the offset it mirrors, and it must do
// so whatever frame the flick starts on (a two-frame mirror/Changed lock is
// phase-dependent, so all eight pre-scroll phases are walked).
func TestScrollRailSettlesAfterFastWheel(t *testing.T) {
	for phase := 0; phase < 8; phase++ {
		tbl, ui, sc := wheelTable(t)
		img := ebiten.NewImage(640, 400)
		frame := func() { tbl.Sync(); ui.Update(); ui.Draw(img) }
		for f := 0; f <= phase; f++ {
			frame()
		}
		for i := 0; i < 2; i++ {
			sc.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -1})
			frame()
		}
		var tops []float64
		for f := 0; f < 30; f++ {
			frame()
			tops = append(tops, sc.ScrollTop)
		}
		for i := 2; i < len(tops); i++ {
			if (tops[i]-tops[i-1])*(tops[i-1]-tops[i-2]) < 0 {
				t.Fatalf("phase %d: offset reversed with no wheel input at frame %d: %.5f -> %.5f -> %.5f",
					phase, i, tops[i-2], tops[i-1], tops[i])
			}
		}
		if got, want := int(math.Round(sc.ScrollTop*RailSteps)), tbl.Rail().Current; got != want {
			t.Fatalf("phase %d: rail settled at %d, want the mirrored offset %d (ScrollTop %.5f)",
				phase, want, got, sc.ScrollTop)
		}
		if sc.ScrollTop <= 0 {
			t.Fatalf("phase %d: the flick scrolled nothing (ScrollTop %.5f)", phase, sc.ScrollTop)
		}
	}
}

// TestScrollRailWheelOverGutterScrolls: a wheel notch over the rail gutter
// is the rail's own gesture — it must still move the list, and settle.
func TestScrollRailWheelOverGutterScrolls(t *testing.T) {
	tbl, ui, sc := wheelTable(t)
	img := ebiten.NewImage(640, 400)
	frame := func() { tbl.Sync(); ui.Update(); ui.Draw(img) }
	frame()
	frame()
	before := sc.ScrollTop
	tbl.Rail().GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -1})
	frame()
	frame()
	if sc.ScrollTop <= before {
		t.Fatalf("wheel over the gutter: ScrollTop = %.5f, want above %.5f", sc.ScrollTop, before)
	}
	var tops []float64
	for f := 0; f < 30; f++ {
		frame()
		tops = append(tops, sc.ScrollTop)
	}
	for i := 2; i < len(tops); i++ {
		if (tops[i]-tops[i-1])*(tops[i-1]-tops[i-2]) < 0 {
			t.Fatalf("gutter wheel: offset reversed with no input at frame %d: %.5f -> %.5f -> %.5f",
				i, tops[i-2], tops[i-1], tops[i])
		}
	}
	if got, want := int(math.Round(sc.ScrollTop*RailSteps)), tbl.Rail().Current; got != want {
		t.Fatalf("gutter wheel: rail settled at %d, want the mirrored offset %d", want, got)
	}
}
