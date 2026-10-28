package game

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The Screen host's own pins: a fake sink records the mount order and the
// committed-close handshake, fixture panels record Sync/Dismissed/
// Unmounted, and no Game/sim/GPU is involved.

type fakeSink struct {
	added   []*widget.Window
	removed []*widget.Window
	closes  map[*widget.Window]func()
	closing map[*widget.Window]bool
	rows    map[*widget.Window]func(image.Point) bool
}

func newFakeSink() *fakeSink {
	return &fakeSink{closes: map[*widget.Window]func(){}, closing: map[*widget.Window]bool{}}
}
func (f *fakeSink) AddWindow(w *widget.Window) widget.RemoveWindowFunc {
	f.added = append(f.added, w)
	return func() { f.removed = append(f.removed, w) }
}
func (f *fakeSink) SetWindowClose(w *widget.Window, close func()) {
	if f.closes == nil {
		f.closes = map[*widget.Window]func(){}
	}
	f.closes[w] = close
}
func (f *fakeSink) Closing(w *widget.Window) bool { return f.closing[w] }
func (f *fakeSink) SetWindowRows(w *widget.Window, hit func(image.Point) bool) {
	if f.rows == nil {
		f.rows = map[*widget.Window]func(image.Point) bool{}
	}
	f.rows[w] = hit
}

type fixturePanel struct {
	win       *widget.Window
	syncs     int
	dismissed int
	unmounted int
	log       []string
	seams     []widgets.Seam
	tables    []*widgets.Table
}

func newFixturePanel() *fixturePanel {
	return &fixturePanel{win: widget.NewWindow(widget.WindowOpts.Contents(widget.NewContainer()))}
}
func (p *fixturePanel) Win() *widget.Window { return p.win }
func (p *fixturePanel) Sync()               { p.syncs++; p.log = append(p.log, "sync") }
func (p *fixturePanel) Dismissed()          { p.dismissed++; p.log = append(p.log, "dismiss") }
func (p *fixturePanel) Unmounted()          { p.unmounted++ }
func (p *fixturePanel) Splits() []widgets.Seam {
	return p.seams
}
func (p *fixturePanel) Tables() []*widgets.Table { return p.tables }

func fixtureRow(id ScreenID, p *fixturePanel, mod func(*Screen)) Screen {
	s := Screen{
		ID: id, Title: string(id), DockPolicy: DockFloating,
		Build: func(*ScreenHost) widgets.WindowPanel { return p },
	}
	if mod != nil {
		mod(&s)
	}
	return s
}

func TestHostWantRules(t *testing.T) {
	rows := []Screen{
		fixtureRow("plain", newFixturePanel(), nil),
		fixtureRow("space", newFixturePanel(), func(s *Screen) { s.SpaceOnly = true }),
		fixtureRow("docked", newFixturePanel(), func(s *Screen) { s.DockedOnly = true }),
		fixtureRow("title", newFixturePanel(), func(s *Screen) { s.InTitle = true }),
	}
	game := newScreenHost(rows, gameScope, newFakeSink(), screenServices{})
	title := newScreenHost(rows, titleScope, newFakeSink(), screenServices{})

	setAll := func(h *ScreenHost, open bool) {
		for _, s := range rows {
			h.SetOpen(s.ID, open)
		}
	}
	check := func(h *ScreenHost, id ScreenID, room Room, want bool) {
		t.Helper()
		if got := h.Want(id, room); got != want {
			t.Errorf("want(%s, %+v) = %v, want %v", id, room, got, want)
		}
	}

	// Closed never mounts.
	check(game, "plain", Room{}, false)
	setAll(game, true)
	// A plain row follows the flag in both rooms; the scope rules filter.
	check(game, "plain", Room{}, true)
	check(game, "plain", Room{Docked: true}, true)
	check(game, "space", Room{}, true)
	check(game, "space", Room{Docked: true}, false)
	check(game, "docked", Room{}, false)
	check(game, "docked", Room{Docked: true}, true)
	check(game, "plain", Room{Title: true}, false) // game host mounts nothing at the title

	setAll(title, true)
	// Title scope: only title rows mount, only while the title room is up;
	// the same rows never mount in gameplay.
	check(title, "title", Room{Title: true}, true)
	check(title, "title", Room{}, false)
	check(title, "plain", Room{Title: true}, false)
}

func TestHostMountLifecycle(t *testing.T) {
	sink := newFakeSink()
	pa, pb := newFixturePanel(), newFixturePanel()
	rows := []Screen{fixtureRow("a", pa, nil), fixtureRow("b", pb, nil)}
	h := newScreenHost(rows, gameScope, sink, screenServices{})
	h.Warm()

	// Warm registers every × before the first mount.
	if len(sink.added) != 0 {
		t.Fatalf("Warm mounted %d windows, want none", len(sink.added))
	}
	for _, w := range []*widget.Window{pa.Win(), pb.Win()} {
		if sink.closes[w] == nil {
			t.Fatalf("window %v has no registered close after Warm", w)
		}
	}

	h.SetOpen("a", true)
	h.Reconcile(Room{})
	if len(sink.added) != 1 || sink.added[0] != pa.Win() {
		t.Fatalf("added = %v, want just a's window", sink.added)
	}
	if pa.syncs != 1 {
		t.Fatalf("mount-time syncs = %d, want 1 (same frame)", pa.syncs)
	}
	h.Reconcile(Room{})
	if len(sink.added) != 1 || pa.syncs != 1 {
		t.Fatalf("second reconcile was not a no-op (added %d, syncs %d)", len(sink.added), pa.syncs)
	}

	// A hotkey close (flag off, no committed fade) unmounts, runs the
	// teardown hook exactly once, and never reports Dismissed.
	h.SetOpen("a", false)
	h.Sync()
	h.Reconcile(Room{})
	if pa.unmounted != 1 || len(sink.removed) != 1 || sink.removed[0] != pa.Win() {
		t.Fatalf("unmount = %d hooks, removed %v", pa.unmounted, sink.removed)
	}
	if pa.dismissed != 0 {
		t.Fatalf("hotkey unmount ran Dismissed %d times", pa.dismissed)
	}
	if h.Mounted("a") || h.Window("a") == nil {
		t.Fatal("unmounted row lost its built window (game host keeps it warm)")
	}
}

func TestHostDismissedOnCommittedCloseOnly(t *testing.T) {
	sink := newFakeSink()
	p := newFixturePanel()
	h := newScreenHost([]Screen{fixtureRow("a", p, nil)}, gameScope, sink, screenServices{})
	h.Warm()
	h.SetOpen("a", true)
	h.Reconcile(Room{})

	// A committed × close: Closing is true before the reconcile; the hook
	// runs once, before that frame's body Sync.
	p.log = nil
	sink.closing[p.Win()] = true
	h.Sync()
	if want := []string{"dismiss", "sync"}; len(p.log) != 2 || p.log[0] != want[0] || p.log[1] != want[1] {
		t.Fatalf("first closing frame log = %v, want %v", p.log, want)
	}
	h.Sync()
	if p.dismissed != 1 {
		t.Fatalf("Dismissed ran %d times, want exactly one per mount cycle", p.dismissed)
	}

	// Unmount + a fresh mount resets the handshake: the next committed
	// close fires again.
	h.SetOpen("a", false)
	h.Reconcile(Room{})
	h.SetOpen("a", true)
	h.Reconcile(Room{})
	h.Sync() // closing[win] is still true: this mount closes at once
	if p.dismissed != 2 {
		t.Fatalf("Dismissed after remount = %d, want 2", p.dismissed)
	}
}

func TestHostTitleScopeIsLazyExclusiveAndFresh(t *testing.T) {
	builds := map[ScreenID]int{}
	mk := func(id ScreenID) *fixturePanel {
		p := newFixturePanel()
		return p
	}
	rows := []Screen{
		fixtureRow("settings", mk("settings"), func(s *Screen) { s.InTitle = true }),
		fixtureRow("keybindings", mk("keybindings"), func(s *Screen) { s.InTitle = true }),
	}
	for i := range rows {
		id := rows[i].ID
		rows[i].Build = func(*ScreenHost) widgets.WindowPanel { builds[id]++; return mk(id) }
	}
	sink := newFakeSink()
	h := newScreenHost(rows, titleScope, sink, screenServices{})
	sink.closing = map[*widget.Window]bool{} // the rebuilt panels' windows are new

	// Lazy: nothing is built before the first open.
	h.Reconcile(Room{Title: true})
	if len(builds) != 0 {
		t.Fatalf("title host built panels before an open: %v", builds)
	}

	h.SetOpen("settings", true)
	h.Reconcile(Room{Title: true})
	if builds["settings"] != 1 || !h.Mounted("settings") {
		t.Fatalf("settings did not mount lazily (builds %v)", builds)
	}
	// Exclusive: opening keybindings closes settings.
	h.SetOpen("keybindings", true)
	h.Reconcile(Room{Title: true})
	if h.IsOpen("settings") || h.Mounted("settings") {
		t.Fatal("opening one title row left the other open")
	}
	if !h.Mounted("keybindings") {
		t.Fatal("keybindings did not mount")
	}
	// Dropped on close: the next open builds a fresh panel.
	h.SetOpen("keybindings", false)
	h.Reconcile(Room{Title: true})
	if h.Panel("keybindings") != nil {
		t.Fatal("closed title row kept its panel")
	}
	h.SetOpen("keybindings", true)
	h.Reconcile(Room{Title: true})
	if builds["keybindings"] != 2 {
		t.Fatalf("reopen did not build a fresh panel (builds %d)", builds["keybindings"])
	}
}

func TestHostRebindKeepsOpenFlags(t *testing.T) {
	pa := newFixturePanel()
	h := newScreenHost([]Screen{fixtureRow("a", pa, nil)}, gameScope, newFakeSink(), screenServices{})
	h.Warm()
	h.SetOpen("a", true)
	h.Reconcile(Room{})

	h.Rebind(newFakeSink(), screenServices{})
	if !h.IsOpen("a") {
		t.Fatal("Rebind dropped the open flag")
	}
	if h.Mounted("a") || h.Panel("a") != nil {
		t.Fatal("Rebind kept a panel/mount from the dropped kit")
	}
	h.Warm()
	if h.Panel("a") == nil {
		t.Fatal("Rebind did not rebuild the panel")
	}
}

func TestHostLayoutFacetsRoundTrip(t *testing.T) {
	// A fixture Splitter and Tabler: the entry implements the facets, so
	// the sidecar helpers see them with no name switch (the old silent
	// no-op class).
	g := newShowcaseGame(t)
	th := theme.NewSeam(theme.DefaultPalette(), theme.ScaleTypeScale(theme.DefaultTypeScale()))
	split := widgets.NewSplitPane(th, widget.NewContainer(), widget.NewContainer())
	seam := widgets.Seam{Name: "fixture-seam", Split: split, Index: 0}
	tbl := widgets.NewTable(g.kit.Env(), []widgets.Column{{Key: "Name", Title: "Name"}}, widgets.PrefsID("fixture"))
	p := newFixturePanel()
	p.seams = []widgets.Seam{seam}
	p.tables = []*widgets.Table{tbl}

	h := newScreenHost([]Screen{fixtureRow("facets", p, nil)}, gameScope, newFakeSink(), screenServices{})
	h.Warm()
	entry := h.Slot("facets")

	// Splits: write side reads the ratio, read side restores it.
	if got := splitRatios(entry)["fixture-seam"]; got != 0.5 {
		t.Fatalf("fresh seam ratio = %v, want 0.5", got)
	}
	applySplits(entry, map[string]float64{"fixture-seam": 0.42})
	if got := split.Ratio(seam.Index); got != 0.42 {
		t.Fatalf("restored seam ratio = %v, want 0.42", got)
	}
	if got := splitRatios(entry)["fixture-seam"]; got != 0.42 {
		t.Fatalf("written seam ratio = %v, want 0.42", got)
	}

	// Tables: write side reads the stored column set, read side applies it.
	prefs := map[string]map[string]widgets.ColumnPref{"fixture": {"Name": {Width: 88}}}
	applyTablePrefs(entry, prefs)
	if got := tablePrefs(entry)["fixture"]["Name"].Width; got != 88 {
		t.Fatalf("restored column width = %v, want 88", got)
	}
	// A panel without the facets resolves nil — the row persists geometry
	// only rather than pretending to have panes/tables.
	plain := h.Slot("facets")
	_ = plain
	other := newScreenHost([]Screen{fixtureRow("plain", newFixturePanel(), nil)}, gameScope, newFakeSink(), screenServices{})
	other.Warm()
	if got := other.Slot("plain").Splits(); got != nil || other.Slot("plain").Tables() != nil {
		t.Fatalf("facetless row exposed facets: %v / %v", got, other.Slot("plain").Tables())
	}
}

func TestHostWindow(t *testing.T) {
	p := newFixturePanel()
	h := newScreenHost([]Screen{fixtureRow("a", p, nil)}, gameScope, newFakeSink(), screenServices{})
	h.Warm()
	if h.Window("a") != p.Win() {
		t.Fatal("Window() is not the Build's return value")
	}
	if h.Window("nope") != nil {
		t.Fatal("unknown id resolved a window")
	}
}
