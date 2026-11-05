package screens

import (
	"image"
	"slices"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// paintLog is a canvas-less widget that records when it is rendered: the
// kit's order is read off the log instead of a pixel (readback needs a
// running game loop).
type paintLog struct {
	w   *widget.Widget
	log *[]string
	tag string
}

func newPaintLog(log *[]string, tag string) *paintLog {
	return &paintLog{w: widget.NewWidget(), log: log, tag: tag}
}

func (p *paintLog) GetWidget() *widget.Widget     { return p.w }
func (p *paintLog) PreferredSize() (int, int)     { return 1, 1 }
func (p *paintLog) SetLocation(r image.Rectangle) { p.w.Rect = r }
func (p *paintLog) Validate()                     {}
func (p *paintLog) Render(*ebiten.Image)          { *p.log = append(*p.log, p.tag) }

// paintTop is the smallest TopChrome: its Draw only records its place in
// the frame's paint order.
type paintTop struct {
	c   *widget.Container
	log *[]string
}

func (p *paintTop) Container() *widget.Container { return p.c }
func (p *paintTop) SetWindowPass(bool)           {}
func (p *paintTop) Draw(*ebiten.Image)           { *p.log = append(*p.log, "rail") }

// TestLayerPaintOrder pins the stacking contract the two layer slots
// declare: the backdrop layer (the proficiency tree) renders BEFORE the
// top chrome, and the front layer (the starmap) renders AFTER it, so only
// the front layer covers the rail.
func TestLayerPaintOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mount func(*Kit, *widget.Window) widget.RemoveWindowFunc
		want  []string
	}{
		{"backdrop paints under the rail", (*Kit).AddBackdropWindow, []string{"layer", "rail"}},
		{"front paints over the rail", (*Kit).AddFrontWindow, []string{"rail", "layer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			k, _ := newTestKit(t)
			k.AddTopContainer(&paintTop{c: widget.NewContainer(), log: &log},
				widget.AnchorLayoutData{
					HorizontalPosition: widget.AnchorLayoutPositionStart,
					VerticalPosition:   widget.AnchorLayoutPositionStart,
				})
			// The layer window's own canvas is what it renders.
			w := widget.NewWindow(widget.WindowOpts.Contents(widget.NewContainer()))
			w.GetContainer().AddChild(newPaintLog(&log, "layer"))
			unmount := tc.mount(k, w)
			pumpKit(t, k, 1280, 720)

			if len(log) < 2 || !slices.Equal(log[:2], tc.want) {
				t.Errorf("paint order = %v, want %v", log, tc.want)
			}
			unmount()
		})
	}
}

// TestMapViewCanvasCross pins the layer's mouse close: the × sits in the
// canvas's upper-right corner, a hover lights it (and repaints), and a
// press runs the row's close — the one toggle Esc and N run.
func TestMapViewCanvasCross(t *testing.T) {
	env, _ := newChromeEnv(t)
	closed := 0
	m := NewMapView(env, nil, func() { closed++ })
	c := newChromeUI(env)
	c.AddWindow(m)
	c.Frame(t, 1280, 720)
	c.Frame(t, 1280, 720)

	r := m.CloseRect()
	w, h := m.img.Bounds().Dx(), m.img.Bounds().Dy()
	if r.Empty() || r.Min.X < w/2 || r.Min.Y < 0 || r.Max.Y > theme.Px(mapHeadH) {
		t.Fatalf("× rect = %v, want a box in the canvas's upper-right header corner (%dx%d)", r, w, h)
	}
	if r.Max.X > w-theme.Px(mapPadOuter) {
		t.Errorf("× rect %v reaches into the outer margin", r)
	}
	for _, p := range []image.Point{r.Min, r.Max.Sub(image.Pt(1, 1)), image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)} {
		if !p.In(r) {
			t.Errorf("× point %v outside its own rect %v", p, r)
		}
	}

	at := func(p image.Point) {
		env.in.x, env.in.y = p.X, p.Y
		env.Pointer().Begin()
	}
	centre := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)

	env.in.edge = [3]bool{false, false, false}
	at(centre)
	m.Sync()
	if !m.overX {
		t.Error("the pointer on the × did not light it")
	}
	if closed != 0 {
		t.Error("a hover closed the layer")
	}

	// Away from the × the hover clears again.
	at(image.Pt(w/2, h/2))
	m.Sync()
	if m.overX {
		t.Error("the × stayed lit with the pointer off it")
	}

	env.in.edge = [3]bool{true, false, false}
	at(centre)
	m.Sync()
	if closed != 1 {
		t.Errorf("the × press ran the close %d times, want 1", closed)
	}
}

// TestMapViewSpacingRhythm pins the layer's spacing rhythm as constants a
// reader can check: the outside-in insets step up (outer margin, column
// inset, card pad), a card's gap is wider than its own padding, every row
// is taller than the glyph it carries, and the zones honour the insets
// they declare.
func TestMapViewSpacingRhythm(t *testing.T) {
	if mapPadOuter < theme.SpLg || mapInset < theme.SpMd || mapCardPad < theme.SpMd {
		t.Errorf("insets = outer %d, column %d, card %d, want at least SpLg/SpMd/SpMd",
			mapPadOuter, mapInset, mapCardPad)
	}
	if mapSection <= mapCardPad {
		t.Errorf("gap between cards %d not wider than the card pad %d", mapSection, mapCardPad)
	}
	if mapBodyH <= icons.Inline || mapRowH <= icons.Inline {
		t.Errorf("row pitches = body %d, browser %d, want both taller than the %d px glyph",
			mapBodyH, mapRowH, icons.Inline)
	}
	if mapHeadGap > mapSection {
		t.Errorf("section head gap %d outruns the card gap %d", mapHeadGap, mapSection)
	}

	env, _ := newChromeEnv(t)
	m := NewMapView(env, nil, nil)
	m.Sync()
	z := m.layout
	pad := theme.Px(mapPadOuter)
	if z.left.Min.X != pad || z.right.Max.X != m.img.Bounds().Dx()-pad {
		t.Errorf("columns %v/%v do not honour the outer margin %d", z.left, z.right, pad)
	}
	if z.left.Min.Y != theme.Px(mapHeadH)+theme.Px(mapInset) {
		t.Errorf("columns start at y %d, want the header band plus the inset", z.left.Min.Y)
	}
	if got := z.graph.Min.X - z.left.Max.X; got != theme.Px(mapGutter) {
		t.Errorf("browser-to-graph gutter = %d, want %d", got, theme.Px(mapGutter))
	}

	// A section card is its two pads, the head band and its bodies.
	sec := overviewSection{head: "BELTS", bodies: []overviewBody{{Kind: content.CelBelt, Ores: []overviewOre{{}, {}}}}}
	want := 2*theme.Px(mapCardPad) + theme.Px(16) + theme.Px(mapHeadGap) + theme.Px(mapBodyH) + 2*theme.Px(mapOreH)
	if got := sectionHeight(sec); got != want {
		t.Errorf("belt card height = %d, want %d (pads, head band, row, two ore chips)", got, want)
	}
}

// TestOverviewSectionsGroupByFamily pins the overview's hierarchy: the
// flat body list becomes one card per kind family, in the canonical order,
// each body landing in its own card.
func TestOverviewSectionsGroupByFamily(t *testing.T) {
	secs := overviewSections(overviewBodies(fixtureSystemDef()))
	wantHead := []string{"CELESTIALS", "STATIONS", "BELTS", "GATES", "ANOMALIES"}
	wantBodies := []int{3, 1, 1, 1, 1}
	if len(secs) != len(wantHead) {
		t.Fatalf("sections = %d, want %d (%v)", len(secs), len(wantHead), secs)
	}
	for i, sec := range secs {
		if sec.head != wantHead[i] {
			t.Errorf("section %d head = %q, want %q", i, sec.head, wantHead[i])
		}
		if len(sec.bodies) != wantBodies[i] {
			t.Errorf("section %q bodies = %d, want %d", sec.head, len(sec.bodies), wantBodies[i])
		}
	}
	// Every body of the fixture is in exactly one card.
	total := 0
	for _, sec := range secs {
		total += len(sec.bodies)
	}
	if total != len(overviewBodies(fixtureSystemDef())) {
		t.Errorf("cards carry %d bodies, want every one of the %d", total, len(overviewBodies(fixtureSystemDef())))
	}

	// Nothing to group: no cards drawn.
	if secs := overviewSections(nil); len(secs) != 0 {
		t.Errorf("empty system made %d section cards", len(secs))
	}
}
