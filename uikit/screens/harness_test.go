package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// NOTE: screens-internal test harness. It may import widgets/theme but
// nothing may import it back, so the shared helpers live here (the
// widgets suite keeps its own seam-only copy; uitest holds the parts
// with no uikit dependency).

// testEnv is what the uikit tests drive panels with: the session (the
// three seams plus its menu and Show Info window) over the embedded
// content and world-showcase, plus the fixture handles — the sim and
// registry the tests write through directly. The panels only ever see
// the Session half: no screen reaches the sim.
type testEnv struct {
	Session
	Sim     *sim.Sim
	Content *content.Content
	in      *scriptPointer // the frame's source: a test moves the pointer here
}

// scriptPointer is a widgets.Source the screens tests drive by hand: the
// session's pointer frame reads it instead of the OS.
type scriptPointer struct {
	x, y   int
	down   [3]bool
	edge   [3]bool
	shape  ebiten.CursorShapeType
	writes int
}

func (s *scriptPointer) Cursor() (int, int)                { return s.x, s.y }
func (s *scriptPointer) Down(b widgets.Button) bool        { return s.down[b] }
func (s *scriptPointer) JustPressed(b widgets.Button) bool { return s.edge[b] }
func (s *scriptPointer) SetShape(sh ebiten.CursorShapeType) {
	s.shape, s.writes = sh, s.writes+1
}

// newTestEnv builds that session, capturing every enqueued command. Its
// pointer frame reads the testEnv's own script (env.in).
func newTestEnv(t *testing.T) (*testEnv, *[]sim.Command) {
	t.Helper()
	in := &scriptPointer{}
	env, cmds := newTestEnvSource(t, widgets.NewPointer(in))
	env.in = in
	return env, cmds
}

// newTestEnvSource builds the session over a caller's frame — a test that
// shares one scripted frame with a Kit (its windows poll the same sample).
func newTestEnvSource(t *testing.T, p *widgets.Pointer) (*testEnv, *[]sim.Command) {
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
	sess := NewEnv(c, s, func(cmd sim.Command) { cmds = append(cmds, cmd) }, p)
	return &testEnv{Session: sess, Sim: s, Content: c}, &cmds
}

// newChromeEnv builds the environment shared by the chrome tests: the
// embedded content, world-showcase at seed 42 and a command-capturing
// enqueue. (newTestKit drives the whole Kit instead.)
func newChromeEnv(t *testing.T) (*testEnv, *[]sim.Command) {
	t.Helper()
	return newTestEnv(t)
}

// chromeUI is the shared chrome test harness: an interface root (anchor
// layout) holding the anchored panels plus any windows, driven through
// Sync → Update → Draw exactly the way the game will drive it.
type chromeUI struct {
	env    widgets.Seams
	ui     *ebitenui.UI
	root   *widget.Container
	panels []widgets.Panel
	wins   []*widget.Window
}

func newChromeUI(env widgets.Seams) *chromeUI {
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	return &chromeUI{
		env:  env,
		ui:   &ebitenui.UI{Container: root, PrimaryTheme: env.WidgetTheme()},
		root: root,
	}
}

// AddPanel attaches an anchored panel's container and its Sync target.
func (c *chromeUI) AddPanel(container *widget.Container, p widgets.Panel) *chromeUI {
	c.root.AddChild(container)
	c.panels = append(c.panels, p)
	return c
}

// AddWindow adds a window panel to the UI.
func (c *chromeUI) AddWindow(w widgets.WindowPanel) *chromeUI {
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
func newWindowUI(t *testing.T, env widgets.Seams, wins ...*widget.Window) *ebitenui.UI {
	t.Helper()
	ui := &ebitenui.UI{Container: widget.NewContainer(), PrimaryTheme: env.WidgetTheme()}
	for _, w := range wins {
		ui.AddWindow(w)
	}
	ui.Update()
	return ui
}

// newQtyUI mounts a panel's windows PLUS the session's quantity picker: a
// bare session only shows the picker (the Kit mounts it through
// Kit.OpenQty), so the tests mount it by hand and treat it as always
// mounted, invisible until an open.
func newQtyUI(t *testing.T, env Session, wins ...*widget.Window) *ebitenui.UI {
	t.Helper()
	return newWindowUI(t, env, append(wins, env.QtyWindow().Win())...)
}

// pumpQty runs one frame with the session's picker synced too — the kit
// does that from Kit.Sync in the game, a bare session's host must.
func pumpQty(t *testing.T, ui *ebitenui.UI, env Session, sync func(), w, h int) {
	t.Helper()
	env.QtyWindow().Sync()
	uitest.PumpWindow(t, ui, sync, w, h)
}

// qtyClick presses the picker's slider at value v through the production
// input path (the session's pointer frame): a press-and-release gesture
// inside the widget, then a frame each side of it. The slider's unit is a
// pixel of its track, so aim at the ends (min/max land exactly) or set mid
// values with Slider.SetValue.
func qtyClick(t *testing.T, ui *ebitenui.UI, env *testEnv, sync func(), v float64) {
	t.Helper()
	pt := sliderPoint(env.QtyWindow().Slider(), v)
	env.in.x, env.in.y = pt.X, pt.Y
	env.in.edge, env.in.down = [3]bool{true, false, false}, [3]bool{true, false, false}
	env.Pointer().Begin()
	pumpQty(t, ui, env, sync, 1024, 700) // rising press: grab + jump to v
	env.in.edge, env.in.down = [3]bool{}, [3]bool{}
	env.Pointer().Begin()
	pumpQty(t, ui, env, sync, 1024, 700) // release: the grab ends
}

// newTestKit builds a Kit over the embedded content and world-showcase,
// capturing every enqueued command. Its pointer frame is a fresh OS one.
func newTestKit(t *testing.T) (*Kit, *[]sim.Command) { return newTestKitWith(t, nil) }

// newTestKitWith builds the kit over a caller's pointer frame (nil = a
// fresh OS one): a test that scripts the pointer shares the frame with the
// panels it mounts.
func newTestKitWith(t *testing.T, p *widgets.Pointer) (*Kit, *[]sim.Command) {
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
	k, err := NewKit(c, s, func(cmd sim.Command) { cmds = append(cmds, cmd) }, WithPointer(p))
	if err != nil {
		t.Fatalf("NewKit: %v", err)
	}
	return k, &cmds
}

// pumpKit runs one frame (Sync, Update, Draw) at the given screen size
// and returns the rendered image: Update dispatches the deferred event
// queue, Draw performs the widget layout pass.
func pumpKit(t *testing.T, k *Kit, w, h int) *ebiten.Image {
	t.Helper()
	k.Pointer().Begin() // the host's one sample per Update
	k.Sync()
	if err := k.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	img := ebiten.NewImage(w, h)
	k.Draw(img)
	return img
}

// clickRow fires a real deferred click on row i and dispatches it on the
// next UI pump (ebitenui queues both handlers and events until Update).
func clickRow(ui *ebitenui.UI, tbl *widgets.Table, i int) {
	tbl.RowWidgetForTest(i).MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Button: ebiten.MouseButtonLeft,
	})
	ui.Update()
}

// rightClickRow fires a row's right-button click (with the widget and an
// offset, so the menu anchors where the cursor was) and dispatches it.
func rightClickRow(t *testing.T, ui *ebitenui.UI, tbl *widgets.Table, i int) {
	t.Helper()
	w := tbl.RowWidgetForTest(i)
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 12,
		OffsetY: 6,
	})
	ui.Update()
}

// rightClick fires a widget's right-button click the way a player
// right-clicks it (anchoring the menu at the cursor offset).
func rightClick(c *chromeUI, t *testing.T, btn *widget.Button) {
	t.Helper()
	w := btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 12,
		OffsetY: 6,
	})
	c.Update(t)
}

// menuHost is what the menu-entry helpers need: the session's menu.
type menuHost interface{ Menu() *widgets.Menu }

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
func entry(t *testing.T, env menuHost, label string) widgets.MenuEntry {
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
	return widgets.MenuEntry{}
}

// pickMenuEntry right-clicks btn open and presses the labelled entry.
func pickMenuEntry(t *testing.T, c *chromeUI, env Session, btn *widget.Button, label string) {
	t.Helper()
	rightClick(c, t, btn)
	if !env.Menu().IsOpen() {
		t.Fatal("right-click opened no menu")
	}
	pressMenuEntry(t, env, label)
}

// tipSection returns a model's section by label (nil when the model
// carries none — absent data is absent, never an empty placeholder).
func tipSection(m widgets.TooltipModel, label string) *widgets.TooltipSection {
	for i := range m.Sections {
		if m.Sections[i].Label == label {
			return &m.Sections[i]
		}
	}
	return nil
}

// tipValue returns one row's value inside a section ("" when either is
// absent).
func tipValue(m widgets.TooltipModel, section, label string) string {
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
func tipSectionLabels(m widgets.TooltipModel) []string {
	out := make([]string, len(m.Sections))
	for i, s := range m.Sections {
		out[i] = s.Label
	}
	return out
}
