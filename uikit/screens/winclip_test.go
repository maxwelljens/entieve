package screens

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// ---------------------------------------------------------------------------
// Frame clip (winclip.go)
// ---------------------------------------------------------------------------

// TestClipWindowBodyWrapsFlatBodies: a mounted flat body is wrapped so its
// paint is confined; a scroll body (its own clip), a body carrying a
// masked-buffer widget (a field), and a re-mount are left alone — the
// wrapper is inserted once and never doubled.
func TestClipWindowBodyWrapsFlatBodies(t *testing.T) {
	k, _ := newTestKit(t)

	help := NewHelp(k.Env(), keyMapRows(6))
	k.AddWindow(help.Win())
	if _, ok := help.Win().Contents.(*winClip); !ok {
		t.Fatalf("flat help body = %T, want *winClip", help.Win().Contents)
	}

	// A flat body with no masked-buffer widget keeps its clip; the order
	// dialog's body carries TextInput fields (MaskedRenderBuffer-backed)
	// and must render unwrapped or the typed text is invisible.
	order := k.OrderWin()
	k.AddWindow(order.Win())
	if _, ok := order.Win().Contents.(*winClip); ok {
		t.Fatal("order body (TextInput fields) must not be wrapped — its masked buffer breaks inside the frame clip")
	}
	// Re-mount (the raise path calls addWindow again): still exactly one
	// wrapper.
	k.AddWindow(help.Win())
	if _, ok := help.Win().Contents.(*winClip); !ok {
		t.Fatalf("re-mount doubled/removed the wrapper: %T", help.Win().Contents)
	}

	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	if _, ok := log.Win().Contents.(*winClip); ok {
		t.Fatal("log body (a TextArea scroll viewport) must not be wrapped — its own clip owns it")
	}

	ov := k.OverviewWindow()
	if _, ok := ov.Contents.(*winClip); ok {
		t.Fatal("overview body (a table scroll viewport) must not be wrapped")
	}
}

// TestHelpGripStopsAtContentMin drives the resize grip on the help window
// through the kit's real frame loop: growing works, but shrinking stops at
// the window's content minimum (the hugged rows + blurb) — never the old
// 400×200 literal that let the two-column table paint past the frame. Run
// at 1× and 1.5× so the floor scales with the type grid.
func TestHelpGripStopsAtContentMin(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		t.Run(fmt.Sprintf("scale%.1f", scale), func(t *testing.T) {
			uitest.UseScale(t, scale)
			k, _ := newTestKit(t)
			h := NewHelp(k.Env(), keyMapRows(37))
			win := h.Win()
			k.AddWindow(win)
			k.CloseInfo()
			// 1600×1200 holds the 1.5× window (≈1350 wide) clear of the
			// edge so a grip band is always reachable.
			pumpKit(t, k, 1600, 1200)

			start := win.GetContainer().GetWidget().Rect
			content := windowMin(win, start)
			if content.Y <= theme.Px(200) {
				t.Fatalf("content floor height = %d, not above the old 200 literal", content.Y)
			}

			fm := useFakeMouse(t)
			frame := func() { pumpKit(t, k, 1600, 1200) }

			// Grow: drag the bottom-right corner outward.
			grow := image.Point{X: start.Max.X - 3, Y: start.Max.Y - 3}
			fm.at(grow.X, grow.Y)
			frame()
			fm.down()
			frame()
			fm.at(grow.X+160, grow.Y+160)
			frame()
			fm.up()
			frame()
			frame()
			grown := win.GetContainer().GetWidget().Rect
			if grown.Dx() <= start.Dx() || grown.Dy() <= start.Dy() {
				t.Fatalf("grip drag did not grow the window: %v -> %v", start, grown)
			}

			// Shrink hard: the floor holds at the content minimum.
			shrink := image.Point{X: grown.Max.X - 3, Y: grown.Max.Y - 3}
			fm.at(shrink.X, shrink.Y)
			frame()
			fm.down()
			frame()
			fm.at(shrink.X-10000, shrink.Y-10000)
			frame()
			fm.up()
			frame()
			frame()

			got := win.GetContainer().GetWidget().Rect
			want := resizeRect(grown, -10000, -10000, edgeRight|edgeBottom, windowMin(win, grown))
			if got != want {
				t.Fatalf("shrink stopped at %v, want %v", got, want)
			}
			if got.Dx() != content.X || got.Dy() != content.Y {
				t.Errorf("floor = %dx%d, want content minimum %dx%d", got.Dx(), got.Dy(), content.X, content.Y)
			}

			// Pin (b): at the floor every content rect sits inside the
			// frame.
			assertBodyInFrame(t, win)
		})
	}
}

// assertBodyInFrame walks a window's subtree and fails if any widget rect
// escapes the window rect. Scroll viewports are skipped (their content is
// intentionally larger and clipped by the viewport itself).
func assertBodyInFrame(t *testing.T, win *widget.Window) {
	t.Helper()
	wr := win.GetContainer().GetWidget().Rect
	var walk func(n widget.PreferredSizeLocateableWidget, depth int)
	walk = func(n widget.PreferredSizeLocateableWidget, depth int) {
		if depth > 0 {
			if r := n.GetWidget().Rect; !r.In(wr) {
				t.Errorf("content %T %v escapes the frame %v", n, r, wr)
			}
		}
		if _, ok := n.(*widget.ScrollContainer); ok {
			return
		}
		if c, ok := n.(widget.Containerer); ok {
			for _, ch := range c.Children() {
				walk(ch, depth+1)
			}
		}
	}
	walk(win.GetContainer(), 0)
}

// ---------------------------------------------------------------------------
// Paint-level frame clip (gated capture)
// ---------------------------------------------------------------------------

// TestWindowClipCapture renders a flat, overflowing body through a real
// ebiten loop and asserts at the pixel level that the clip keeps every
// drawn pixel inside the window frame (with glyph pixels inside, so the
// check is not vacuous). Skipped unless ENTIEVE_CAPTURE=1.
func TestWindowClipCapture(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	env, _ := newChromeEnv(t)
	win := newOverflowWindow(env, 40)
	clipWindowBody(win)
	ui := newWindowUI(t, env, win)

	g := &clipCaptureGame{ui: ui, win: win, out: filepath.Join(outDir, "window_clip.png")}
	uitest.RunCaptureLoop(t, "Entieve window clip capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// newOverflowWindow builds a title-less window whose flat body is a 40-row
// vertical stack far taller than the frame it is given, with a deliberately
// small MinSize so the body really overflows the shrunk frame.
func newOverflowWindow(env widgets.Env, rows int) *widget.Window {
	p := env.Pal()
	body := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		widget.RowLayoutOpts.Spacing(theme.Px(4)),
		widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
	)))
	for i := 0; i < rows; i++ {
		body.AddChild(widget.NewText(
			widget.TextOpts.Text(fmt.Sprintf("overflow row %d", i), env.Face(theme.StepSmall), theme.Solid(p.Text)),
		))
	}
	return widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.MinSize(theme.Px(120), theme.Px(80)),
		widget.WindowOpts.Location(image.Rect(40, 40, 500, 320)),
	)
}

type clipCaptureGame struct {
	ui interface {
		Update()
		Draw(*ebiten.Image)
	}
	win    *widget.Window
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *clipCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	if g.frames == 2 {
		// Shrink far below the content: the frame clip must confine the
		// overflowing rows. SetLocation is the outer boundary (ebitenui's
		// own MinSize clamp); the grip clamp is pinned separately.
		g.win.SetLocation(image.Rect(40, 40, 260, 180))
	}
	g.ui.Update()
	g.frames++
	if g.frames > 5 && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *clipCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(800, 600)
	g.ui.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
}

func (g *clipCaptureGame) Layout(w, h int) (int, int) { return 800, 600 }

func (g *clipCaptureGame) verify(img *ebiten.Image) {
	f, err := os.Create(g.out)
	if err != nil {
		g.err = err
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		g.err = err
		return
	}
	g.saved = true

	win := g.win.GetContainer().GetWidget().Rect
	leak, in := 0, 0
	var leakBox image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if x >= win.Min.X && x < win.Max.X && y >= win.Min.Y && y < win.Max.Y {
				in++
				continue
			}
			px := image.Rect(x, y, x+1, y+1)
			if leak == 0 {
				leakBox = px
			} else {
				leakBox = leakBox.Union(px)
			}
			leak++
		}
	}
	if leak != 0 {
		g.err = fmt.Errorf("window %v: %d pixels drawn outside the frame (leak box %v)", win, leak, leakBox)
		return
	}
	if in == 0 {
		g.err = fmt.Errorf("window %v: no pixels inside the frame — the check is vacuous", win)
	}
}
