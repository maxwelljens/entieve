package game

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// The Screen spec's own tests: the well-formedness rule (a malformed row
// must fail loudly — missing builder, duplicate id/key/name, zero title,
// a transient screen carrying a sidecar name, …), the derivation pins
// (every registry equals what the spec derives), the placement contract
// (what each Anchor means, independent of the rows) and the one-row
// demonstration (a fixture row derives an enum slot, a KeyMap binding, a
// sidecar name and a default placement with no other edit).

// withFixture appends one well-formed fixture row to a copy of the spec —
// the "new window" the scalability demonstration adds.
func withFixture() []Screen {
	fixture := Screen{
		ID: panelID(len(screens)), Title: "Fixture", Icon: "svc_inventory",
		Key: ebiten.KeyV, Action: "fixture", Desc: "V — toggle fixture",
		DefaultRect: image.Rect(0, 0, defaultW, defaultH),
		DockPolicy:  DockFloating, SidecarName: "fixture", Group: "Trade",
		Build: func(*Game) *widget.Window { return nil },
	}
	return append(slices.Clone(screens), fixture)
}

// TestScreenSpecWellFormed: the production table passes the spec's own
// rule (New runs the same check), so no session can be built on a row
// that would silently never mount, never persist or never document its key.
func TestScreenSpecWellFormed(t *testing.T) {
	if probs := screenSpecErrors(screens); len(probs) > 0 {
		t.Fatalf("production Screen spec is malformed:\n  %s", strings.Join(probs, "\n  "))
	}
	// The fixture row is well-formed too: adding a window is one row.
	if probs := screenSpecErrors(withFixture()); len(probs) > 0 {
		t.Fatalf("fixture row is malformed:\n  %s", strings.Join(probs, "\n  "))
	}
}

// TestScreenSpecRejectsMalformedRows: every way a row can rot is
// reported — each mutation below is a mistake a future edit could make,
// and screenSpecErrors has to name it (the tables pin the message the
// spec test greps for, so a rule cannot quietly disappear).
func TestScreenSpecRejectsMalformedRows(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(rows []Screen)
		want   string
	}{
		{"missing builder", func(r []Screen) { r[0].Build = nil }, "missing builder"},
		{"duplicate id", func(r []Screen) { r[1].ID = pInventory }, "already used by"},
		{"ids out of enum order", func(r []Screen) { r[0], r[1] = r[1], r[0] }, "out of order"},
		{"duplicate key", func(r []Screen) { r[1].Key = ebiten.KeyI }, "already bound by"},
		{"zero title", func(r []Screen) { r[0].Title = "" }, "zero title"},
		{"unknown icon", func(r []Screen) { r[0].Icon = "svc_nope" }, "unknown icon"},
		{"key without an action id", func(r []Screen) { r[0].Action = "" }, "without an action id"},
		{"duplicate action id", func(r []Screen) { r[1].Action = "inventory" }, "already used by"},
		{"help text without a key", func(r []Screen) { r[0].Key = keyNone }, "help text without a key binding"},
		{"desc without the em dash", func(r []Screen) { r[0].Desc = "toggle inventory" }, "must read"},
		{"transient screen with a name", func(r []Screen) { r[pMenu].SidecarName = "menu" }, "transient screen must not carry"},
		{"persisted screen without a name", func(r []Screen) { r[0].SidecarName = "" }, "needs a sidecar name"},
		{"duplicate sidecar name", func(r []Screen) { r[1].SidecarName = "inventory" }, `sidecar name "inventory" already used`},
		{"station screen outside the composition", func(r []Screen) { r[pMarket].SidecarName = "bourse" }, "no composed rect"},
		{"missing rail group", func(r []Screen) { r[0].Group = "" }, "missing rail group"},
		{"unknown rail group", func(r []Screen) { r[0].Group = "Cargo Bay" }, "unknown rail group"},
		{"transient row claiming a group", func(r []Screen) { r[pMenu].Group = "Ship" }, "transient screen must not carry"},
		{"unknown station service", func(r []Screen) { r[1].Service = "banking" }, "unknown station service"},
		{"row without a rail icon", func(r []Screen) { r[0].Icon = "" }, "missing icon"},
		{"space-only and docked-only at once", func(r []Screen) { r[pFactory].SpaceOnly = true }, "never mounts"},
		{"unknown anchor", func(r []Screen) { r[0].Anchor = Anchor(99) }, "unknown anchor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := slices.Clone(screens)
			tc.mutate(rows)
			probs := screenSpecErrors(rows)
			for _, p := range probs {
				if strings.Contains(p, tc.want) {
					return
				}
			}
			t.Errorf("screenSpecErrors = %q, want one mentioning %q", probs, tc.want)
		})
	}
}

// TestScreenSpecMatchesRegistries: every registry equals its derivation
// from the spec — the panelDef table (names, windows, flags, mount
// rules), the sidecar name list, the docked trio and the test label map.
// (The enum and its count are pinned by TestPanelDefsCoverEnum, KeyMap
// and the help rows by TestKeyMapScreenRowsDeriveFromSpec, the default
// rects by TestScreenPlacementDerivesFromSpec.)
func TestScreenSpecMatchesRegistries(t *testing.T) {
	g := newShowcaseGame(t)
	for _, s := range screens {
		d := panelDefs[s.ID]
		if d.name != s.SidecarName {
			t.Errorf("%s: panelDef name = %q, want %q", screenLabel(s), d.name, s.SidecarName)
		}
		if w := d.window(g); w == nil || w != g.screenWin[s.ID] {
			t.Errorf("%s: panelDef window = %v, want the built %v", screenLabel(s), w, g.screenWin[s.ID])
		}
		if o := d.open(g); o != &g.open[s.ID] {
			t.Errorf("%s: panelDef open flag is not the row's", screenLabel(s))
		}
		if (d.sync == nil) != (s.Sync == nil) {
			t.Errorf("%s: panelDef sync presence (%v) differs from the row's (%v)",
				screenLabel(s), d.sync != nil, s.Sync != nil)
		}
		if d.sync != nil {
			d.sync(g) // the row's refresh runs against a live session
		}
		// Mount rule in both views, read back as behaviour: a SpaceOnly
		// screen never mounts docked, a DockedOnly screen never mounts in
		// space, everything else follows its flag.
		g.open[s.ID] = true
		switch {
		case s.SpaceOnly:
			if !d.want(g, false) || d.want(g, true) {
				t.Errorf("%s: SpaceOnly want(space, docked) = (%v, %v), want (true, false)",
					screenLabel(s), d.want(g, false), d.want(g, true))
			}
		case s.DockedOnly:
			if d.want(g, false) || !d.want(g, true) {
				t.Errorf("%s: DockedOnly want(space, docked) = (%v, %v), want (false, true)",
					screenLabel(s), d.want(g, false), d.want(g, true))
			}
		default:
			if !d.want(g, false) || !d.want(g, true) {
				t.Errorf("%s: want(space, docked) = (%v, %v), want (true, true)",
					screenLabel(s), d.want(g, false), d.want(g, true))
			}
		}
		g.open[s.ID] = false
	}

	// Sidecar: the core windows plus every named row, in spec order.
	wantNames := append([]string{"overview", "selected", "infowin"}, sidecarNames(screens)...)
	if !slices.Equal(layoutNames, wantNames) {
		t.Errorf("layoutNames = %v, want %v", layoutNames, wantNames)
	}

	// Docked trio: one membership, read by scenes.go and by the layout.
	if !slices.Equal(stationPanels, stationScreens(screens)) {
		t.Errorf("stationPanels = %v, want %v", stationPanels, stationScreens(screens))
	}
	for _, s := range screens {
		if (s.DockPolicy == DockStation) != slices.Contains(stationPanels, s.ID) {
			t.Errorf("%s: DockPolicy %d disagrees with trio membership", screenLabel(s), s.DockPolicy)
		}
	}

	// Test labels: draw_test's panelNames is the spec's label set.
	if len(panelNames) != len(screens) {
		t.Errorf("panelNames covers %d ids, want %d", len(panelNames), len(screens))
	}
	for _, s := range screens {
		if got := panelNames[s.ID]; got != screenLabel(s) {
			t.Errorf("panelNames[%d] = %q, want %q", s.ID, got, screenLabel(s))
		}
	}
}

// TestScreenPlacementDerivesFromSpec: the composed default rect of every
// screen equals the spec's derivation (DefaultRect + Anchor, or the
// station composition), and the anchors' observable meaning — pinned
// independently of the rows, so a row that forgets its Anchor fails here
// instead of silently centring.
func TestScreenPlacementDerivesFromSpec(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		w, h := size[0], size[1]
		g := newShowcaseGame(t)
		g.applySize(w, h)
		lay := stationLayout(w, h, g.svcWin.GetContainer().GetWidget().Rect.Dy())

		for _, s := range screens {
			r := g.screenWin[s.ID].GetContainer().GetWidget().Rect
			var want image.Rectangle
			if s.DockPolicy == DockStation {
				var ok bool
				if want, ok = lay.forName(s.SidecarName); !ok {
					t.Fatalf("%s: no composed rect for %q", screenLabel(s), s.SidecarName)
				}
			} else {
				want = anchorRect(s.DefaultRect, s.Anchor, r.Dx(), r.Dy(), w, h)
			}
			if r != want {
				t.Errorf("%dx%d: %s = %v, want the derived %v", w, h, screenLabel(s), r, want)
			}
		}

		// Anchor semantics, stated as behaviour (the pre-spec contract).
		// The log's pocket starts clear of the rail's gutter (46 = 10 +
		// uikit.RailInset) and keeps its 288 px width and bottom inset.
		if r := g.screenWin[pLog].GetContainer().GetWidget().Rect; r != image.Rect(46, h-10-240, 46+288, h-10) {
			t.Errorf("%dx%d: log = %v, want the bottom-left pocket (46, %d)-(334, %d)",
				w, h, r, h-10-240, h-10)
		}
		// The scanner sits in the centre column, 36 px right of the
		// pre-rail design position (396 + uikit.RailInset).
		if r := g.screenWin[pScanner].GetContainer().GetWidget().Rect; r.Min != image.Pt(432, 276) {
			t.Errorf("%dx%d: scanner at %v, want the fixed design position (432,276)", w, h, r.Min)
		}
		if r := g.screenWin[pHelp].GetContainer().GetWidget().Rect; r.Min.Y != 24 || r.Min.X != (w-r.Dx())/2 {
			t.Errorf("%dx%d: help at %v, want top inset 24 and x centred (%d)", w, h, r.Min, (w-r.Dx())/2)
		}
		for _, p := range []panelID{pMap, pMenu, pFactory, pSettings} {
			r := g.screenWin[p].GetContainer().GetWidget().Rect
			if want := centerRect(r.Dx(), r.Dy(), w, h); r.Min != want.Min {
				t.Errorf("%dx%d: %s at %v, want centred at %v", w, h, panelNames[p], r.Min, want.Min)
			}
		}
	}
}

// TestScreenKeysToggleTheirPanels: every keyed, non-modal row flips its
// own flag from the spec (the modal rows are scene-owned: handleKeys
// routes Esc/N/H to their scenes). A new row's key works without touching
// handleKeys.
func TestScreenKeysToggleTheirPanels(t *testing.T) {
	g := newShowcaseGame(t)
	for _, s := range screens {
		if s.Modal || s.Key == keyNone {
			continue
		}
		press := Input{JustPressed: []ebiten.Key{s.Key}}
		if s.Shift {
			press.KeysDown = []ebiten.Key{ebiten.KeyShift}
		}
		before := g.open[s.ID]
		if err := g.handleKeys(press); err != nil {
			t.Fatalf("%s: handleKeys: %v", screenLabel(s), err)
		}
		if g.open[s.ID] == before {
			t.Errorf("%s: %v did not toggle the flag (still %v)", screenLabel(s), s.Key, before)
		}
		if err := g.handleKeys(press); err != nil {
			t.Fatalf("%s: handleKeys (release): %v", screenLabel(s), err)
		}
		if g.open[s.ID] != before {
			t.Errorf("%s: second press did not toggle back (now %v)", screenLabel(s), g.open[s.ID])
		}
	}
}

// TestOneScreenRowDerivesEveryRegistry: the scalability proof. One
// fixture row — the whole declaration a new window needs — produces an
// enum slot, a KeyMap binding (that coexists with the world bindings), a
// sidecar name, a default placement and a registry row, with no other
// edit. The derivations run over a copy of the table because the runtime
// registries are sized by the panel enum (arrays), so a fixture id cannot
// join a live session — these are the same functions init derives the
// production registries from.
func TestOneScreenRowDerivesEveryRegistry(t *testing.T) {
	rows := withFixture()
	fixture := rows[len(rows)-1]

	// Enum: the row slots in after the last id, still in order.
	if int(fixture.ID) != len(rows)-1 {
		t.Errorf("fixture id = %d, want %d (contiguous after the enum)", fixture.ID, len(rows)-1)
	}
	for i, s := range rows {
		if s.ID != panelID(i) {
			t.Fatalf("rows[%d].ID = %d, want %d", i, s.ID, i)
		}
	}

	// panelDefs: the new row registers (name, window and flag accessors).
	defs := derivePanelDefs(rows)
	if len(defs) != len(rows) {
		t.Fatalf("derivePanelDefs = %d rows, want %d", len(defs), len(rows))
	}
	if d := defs[fixture.ID]; d.name != "fixture" || d.window == nil || d.open == nil || d.want == nil {
		t.Errorf("fixture panelDef = %+v, want a complete row named fixture", d)
	}

	// KeyMap: the row's binding derives, and still never collides.
	binding := Binding{Key: fixture.Key, Action: fixture.Action, Desc: fixture.Desc}
	bindings := slices.Concat(worldKeyMap, screenBindings(rows))
	if n := countBinding(bindings, binding); n != 1 {
		t.Errorf("fixture binding appears %d times in the derived KeyMap, want 1", n)
	}
	seen := map[Binding]bool{}
	for _, b := range bindings {
		if seen[b] {
			t.Fatalf("derived KeyMap carries a duplicate binding: %+v", b)
		}
		seen[b] = true
	}

	// Sidecar: the name joins the persisted list.
	if !slices.Contains(sidecarNames(rows), "fixture") {
		t.Errorf("sidecarNames = %v, want it to contain fixture", sidecarNames(rows))
	}
	// Default placement: a centred row composes a rect on screen.
	r := anchorRect(fixture.DefaultRect, fixture.Anchor, 300, 200, 1280, 720)
	if want := centerRect(300, 200, 1280, 720); r != want {
		t.Errorf("fixture placement = %v, want centred %v", r, want)
	}
	// Test labels follow the sidecar key.
	if label := screenLabel(fixture); label != "fixture" {
		t.Errorf("fixture label = %q, want fixture", label)
	}
}

// countBinding counts exact occurrences of b in bindings.
func countBinding(bindings []Binding, b Binding) int {
	n := 0
	for _, k := range bindings {
		if k == b {
			n++
		}
	}
	return n
}
