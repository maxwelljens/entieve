package widgets

import (
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// NOTE: widgets-internal test harness. A widgets test may import
// theme — never screens — so this seam-only env builds the three seams
// (theme + view + executor) with its own context menu and no Show Info
// window; the integration tests that drive the popup services live in
// the screens suite.

// testEnv is what the widgets tests drive tables and menus with: the
// three seams over the embedded content and world-showcase plus the
// fixture handles, and the suite's own context menu (the seam-only
// stand-in for the session services).
type testEnv struct {
	Seams
	Sim     *sim.Sim
	Content *content.Content
	menu    *Menu
	in      *scriptInput // the frame's source: a test moves the pointer here
}

// Menu returns the env's context menu (the session-shaped accessor the
// menu/row-menu tests drive).
func (e *testEnv) Menu() *Menu { return e.menu }

// OpenRowMenu derives the menu for spec and opens it at at — the
// installer the session's version wraps.
func (e *testEnv) OpenRowMenu(spec RowSpec, at image.Point) {
	OpenRowMenu(e, e, spec, at)
}

// newTestEnv builds the seam-only env, capturing every enqueued command.
func newTestEnv(t *testing.T) (*testEnv, *[]sim.Command) {
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
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	in := &scriptInput{}
	frame := NewPointer(in)
	m := NewMenu(th, frame)
	// The built-in executor with no Show Info host: session intents land
	// on the env's own menu, command intents tail onto enqueue.
	exec := NewExec(
		nil,
		func(entries []MenuEntry, at image.Point) { m.Open(entries, at) },
		func(cmd sim.Command) { cmds = append(cmds, cmd) },
		nil,
	)
	env := &testEnv{
		Seams:   NewSeams(th, NewView(c, s), exec, frame),
		Sim:     s,
		Content: c,
		menu:    m,
		in:      in,
	}
	return env, &cmds
}

// newChromeEnv builds the environment shared by the chrome tests (the
// seam-only env; the chrome panels themselves come from this package).
func newChromeEnv(t *testing.T) (*testEnv, *[]sim.Command) {
	t.Helper()
	return newTestEnv(t)
}

// chromeUI is the shared chrome test harness: an interface root (anchor
// layout) holding the anchored panels plus any windows, driven through
// Sync → Update → Draw exactly the way the game will drive it.
type chromeUI struct {
	env    Seams
	ui     *ebitenui.UI
	root   *widget.Container
	panels []Panel
	wins   []*widget.Window
}

func newChromeUI(env Seams) *chromeUI {
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	return &chromeUI{
		env:  env,
		ui:   &ebitenui.UI{Container: root, PrimaryTheme: env.WidgetTheme()},
		root: root,
	}
}

// AddPanel attaches an anchored panel's container and its Sync target.
func (c *chromeUI) AddPanel(container *widget.Container, p Panel) *chromeUI {
	c.root.AddChild(container)
	c.panels = append(c.panels, p)
	return c
}

// AddWindow adds a window panel to the UI.
func (c *chromeUI) AddWindow(w WindowPanel) *chromeUI {
	c.ui.AddWindow(w.Win())
	c.panels = append(c.panels, w)
	c.wins = append(c.wins, w.Win())
	return c
}

// Update runs one UI update (this is what flushes ebitenui's deferred
// event queue: handler registration, event delivery).
func (c *chromeUI) Update(t *testing.T) {
	t.Helper()
	c.ui.Update()
}

// Frame runs one full Sync/Update/Draw cycle at w×h.
func (c *chromeUI) Frame(t *testing.T, w, h int) *ebiten.Image {
	t.Helper()
	img := ebiten.NewImage(w, h)
	for _, p := range c.panels {
		p.Sync()
	}
	c.ui.Update()
	c.ui.Draw(img)
	return img
}

// Smoke draws two frames at both design sizes and asserts every window
// stays inside the screen.
func (c *chromeUI) Smoke(t *testing.T) {
	t.Helper()
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		c.Frame(t, size[0], size[1])
		c.Frame(t, size[0], size[1])
		for _, w := range c.wins {
			r := w.GetContainer().GetWidget().Rect
			if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > size[0] || r.Max.Y > size[1] {
				t.Errorf("%dx%d: window %v out of bounds", size[0], size[1], r)
			}
		}
	}
}

// newWindowUI mounts windows on their own UI (the kit's AddWindow seam
// does the same on the kit's UI) and pumps once so the deferred
// constructor handlers register.
func newWindowUI(t *testing.T, env Seams, wins ...*widget.Window) *ebitenui.UI {
	t.Helper()
	ui := &ebitenui.UI{Container: widget.NewContainer(), PrimaryTheme: env.WidgetTheme()}
	for _, w := range wins {
		ui.AddWindow(w)
	}
	ui.Update()
	return ui
}

// newPumpedTable builds a standalone table in its own UI and pumps one
// frame so the deferred constructor handlers are registered and the
// widget layout has run at 640×400. Options pass through to NewTable.
func newPumpedTable(t *testing.T, env Seams, cols []Column, opts ...TableOption) (*Table, *ebitenui.UI) {
	t.Helper()
	tbl := NewTable(env, cols, opts...)
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update()
	ui.Draw(ebiten.NewImage(640, 400))
	return tbl, ui
}

// clickRow fires a real deferred click on row i and dispatches it on the
// next UI pump (ebitenui queues both handlers and events until Update).
func clickRow(ui *ebitenui.UI, tbl *Table, i int) {
	tbl.RowWidgetForTest(i).MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Button: ebiten.MouseButtonLeft,
	})
	ui.Update()
}

// menuHost is what the menu-entry helpers need: the env's menu.
type menuHost interface{ Menu() *Menu }

// clickMenuEntry clicks the labelled entry in the open menu (fails when
// there is no menu open or no such entry).
func clickMenuEntry(t *testing.T, env menuHost, label string) {
	t.Helper()
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatalf("no menu open (want the %q entry)", label)
	}
	for i, e := range m.Entries() {
		if e.Label != label {
			continue
		}
		m.Click(m.RowRect(i).Min.Add(image.Pt(1, 1)))
		return
	}
	t.Fatalf("menu has no %q entry: %+v", label, m.Entries())
}

// pressMenuEntry picks the labelled entry in the already-open menu and
// fails when it is disabled.
func pressMenuEntry(t *testing.T, env menuHost, label string) {
	t.Helper()
	if e := entry(t, env, label); e.Disabled {
		t.Fatalf("menu entry %q is disabled", label)
	}
	clickMenuEntry(t, env, label)
}

// entry returns the menu entry with the label (fails when missing).
func entry(t *testing.T, env menuHost, label string) MenuEntry {
	t.Helper()
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatalf("no menu open (want the %q entry)", label)
	}
	for _, e := range m.Entries() {
		if e.Label == label {
			return e
		}
	}
	t.Fatalf("menu has no %q entry: %+v", label, m.Entries())
	return MenuEntry{}
}

// pickMenuEntry right-clicks btn open and presses the labelled entry.
func pickMenuEntry(t *testing.T, c *chromeUI, env menuHost, btn *widget.Button, label string) {
	t.Helper()
	w := btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 12,
		OffsetY: 6,
	})
	c.Update(t)
	if !env.Menu().IsOpen() {
		t.Fatal("right-click opened no menu")
	}
	pressMenuEntry(t, env, label)
}

// tipSection returns a model's section by label (nil when the model
// carries none — absent data is absent, never an empty placeholder).
func tipSection(m TooltipModel, label string) *TooltipSection {
	for i := range m.Sections {
		if m.Sections[i].Label == label {
			return &m.Sections[i]
		}
	}
	return nil
}

// tipValue returns one row's value inside a section ("" when either is
// absent).
func tipValue(m TooltipModel, section, label string) string {
	sec := tipSection(m, section)
	if sec == nil {
		return ""
	}
	for _, row := range sec.Rows {
		if row.Label == label {
			return row.Value
		}
	}
	return ""
}

// tipSectionLabels renders a model's section labels for failure messages.
func tipSectionLabels(m TooltipModel) []string {
	out := make([]string, len(m.Sections))
	for i, s := range m.Sections {
		out[i] = s.Label
	}
	return out
}
