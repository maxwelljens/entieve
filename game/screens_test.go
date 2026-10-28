package game

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The Screen spec's own tests: the well-formedness rule (a malformed row
// must fail loudly — missing builder, duplicate id/key/name, zero title,
// a transient screen carrying a sidecar name, …), the derivation pins
// (every registry equals what the spec derives), the placement contract
// (what each Anchor means, independent of the rows) and the one-row
// demonstration (a fixture row derives an enum slot, a KeyMap binding, a
// sidecar name and a default placement with no other edit).

// screenIndex / screenByID resolve the spec rows by their frozen ids for
// tests that used to index the old enum.
func screenIndex(id ScreenID) int {
	for i, s := range screens {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func screenByID(id ScreenID) Screen { return screens[screenIndex(id)] }

// withFixture appends one well-formed fixture row to a copy of the spec —
// the "new window" the scalability demonstration adds.
func withFixture() []Screen {
	fixture := Screen{
		ID: "fixture", Title: "Fixture", Icon: "svc_inventory",
		Key: ebiten.KeyV, Action: "fixture", Desc: "toggle fixture",
		DefaultRect: image.Rect(0, 0, defaultW, defaultH),
		DockPolicy:  DockFloating, Group: "Trade",
		Build: func(*ScreenHost) widgets.WindowPanel { return nil },
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
		{"duplicate id", func(r []Screen) { r[1].ID = idInventory }, "already used by"},
		{"zero id", func(r []Screen) { r[0].ID = "" }, "zero id"},
		{"upper-case id", func(r []Screen) { r[0].ID = "Inventory" }, "lowercase sidecar key"},
		{"duplicate key", func(r []Screen) { r[1].Key = ebiten.KeyI }, "already bound by"},
		{"zero title", func(r []Screen) { r[0].Title = "" }, "zero title"},
		{"unknown icon", func(r []Screen) { r[0].Icon = "svc_nope" }, "unknown icon"},
		{"key without an action id", func(r []Screen) { r[0].Action = "" }, "without an action id"},
		{"duplicate action id", func(r []Screen) { r[1].Action = "inventory" }, "already used by"},
		{"help text without a key", func(r []Screen) { r[0].Key = keyNone }, "help text without a key binding"},
		{"desc with a baked key", func(r []Screen) { r[0].Desc = "I — toggle inventory" }, "carries a chord"},
		{"missing rail group", func(r []Screen) { r[0].Group = "" }, "missing rail group"},
		{"unknown rail group", func(r []Screen) { r[0].Group = "Cargo Bay" }, "unknown rail group"},
		{"transient row claiming a group", func(r []Screen) { r[screenIndex(idMenu)].Group = "Ship" }, "transient screen must not carry"},
		{"unknown station service", func(r []Screen) { r[1].Service = "banking" }, "unknown station service"},
		{"row without a rail icon", func(r []Screen) { r[0].Icon = "" }, "missing icon"},
		{"space-only and docked-only at once", func(r []Screen) { r[screenIndex(idManufacturing)].SpaceOnly = true }, "never mounts"},
		{"unknown anchor", func(r []Screen) { r[0].Anchor = Anchor(99) }, "unknown anchor"},
		{"duplicate title", func(r []Screen) { r[1].Title = "Inventory" }, "title already used by"},
		{"unknown dock policy", func(r []Screen) { r[0].DockPolicy = DockPolicy(99) }, "unknown dock policy"},
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
		if w := g.host.Window(s.ID); w == nil {
			t.Errorf("%s: host has no built window", screenLabel(s))
		}
		if p := g.host.Panel(s.ID); p == nil || p.Win() != g.host.Window(s.ID) {
			t.Errorf("%s: host panel window disagrees with Window()", screenLabel(s))
		}
		// Mount rule in both views, read back as behaviour: a SpaceOnly
		// screen never mounts docked, a DockedOnly screen never mounts in
		// space, everything else follows its flag.
		g.host.SetOpen(s.ID, true)
		switch {
		case s.SpaceOnly:
			if !g.host.Want(s.ID, Room{}) || g.host.Want(s.ID, Room{Docked: true}) {
				t.Errorf("%s: SpaceOnly want(space, docked) = (%v, %v), want (true, false)",
					screenLabel(s), g.host.Want(s.ID, Room{}), g.host.Want(s.ID, Room{Docked: true}))
			}
		case s.DockedOnly:
			if g.host.Want(s.ID, Room{}) || !g.host.Want(s.ID, Room{Docked: true}) {
				t.Errorf("%s: DockedOnly want(space, docked) = (%v, %v), want (false, true)",
					screenLabel(s), g.host.Want(s.ID, Room{}), g.host.Want(s.ID, Room{Docked: true}))
			}
		default:
			if !g.host.Want(s.ID, Room{}) || !g.host.Want(s.ID, Room{Docked: true}) {
				t.Errorf("%s: want(space, docked) = (%v, %v), want (true, true)",
					screenLabel(s), g.host.Want(s.ID, Room{}), g.host.Want(s.ID, Room{Docked: true}))
			}
		}
		g.host.SetOpen(s.ID, false)
	}

	// Sidecar: the core windows plus every persisted row, in spec order.
	wantNames := append(coreLayoutNames(), sidecarNames(screens)...)
	if !slices.Equal(layoutNames, wantNames) {
		t.Errorf("layoutNames = %v, want %v", layoutNames, wantNames)
	}

	// Test labels: draw_test's panelNames is the spec's label set.
	if len(panelNames) != len(screens) {
		t.Errorf("panelNames covers %d ids, want %d", len(panelNames), len(screens))
	}
	for _, s := range screens {
		if got := panelNames[s.ID]; got != screenLabel(s) {
			t.Errorf("panelNames[%q] = %q, want %q", s.ID, got, screenLabel(s))
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

		for _, s := range screens {
			r := g.host.Window(s.ID).GetContainer().GetWidget().Rect
			want := anchorRect(s.DefaultRect, s.Anchor, r.Dx(), r.Dy(), w, h)
			if r != want {
				t.Errorf("%dx%d: %s = %v, want the derived %v", w, h, screenLabel(s), r, want)
			}
		}

		// Anchor semantics, stated as behaviour (the pre-spec contract).
		// The log's pocket starts clear of the rail's gutter (97 = 10 +
		// uikit.RailInset) and keeps its 264 px width and bottom inset.
		if r := g.host.Window(idLog).GetContainer().GetWidget().Rect; r != image.Rect(97, h-10-240, 97+264, h-10) {
			t.Errorf("%dx%d: log = %v, want the bottom-left pocket (97, %d)-(361, %d)",
				w, h, r, h-10-240, h-10)
		}
		// The scanner sits in the centre column, 87 px right of the
		// pre-rail design position (396 + uikit.RailInset).
		if r := g.host.Window(idScanner).GetContainer().GetWidget().Rect; r.Min != image.Pt(483, 276) {
			t.Errorf("%dx%d: scanner at %v, want the fixed design position (483,276)", w, h, r.Min)
		}
		// The help window centres on top (24 px inset) with its left
		// edge floored clear of the rail gutter: centred lands inside
		// the 87 px rail at the 1024 minimum, so the floor (10+RailInset
		// = 97) wins there.
		if r := g.host.Window(idHelp).GetContainer().GetWidget().Rect; r.Min.Y != 24 || r.Min.X != max(railClearX(), (w-r.Dx())/2) {
			t.Errorf("%dx%d: help at %v, want top inset 24 and x = max(%d, centred %d)",
				w, h, r.Min, railClearX(), (w-r.Dx())/2)
		}
		for _, p := range []ScreenID{idMap, idMenu, idManufacturing, idSettings} {
			r := g.host.Window(p).GetContainer().GetWidget().Rect
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
		before := g.host.IsOpen(s.ID)
		if err := g.handleKeys(press); err != nil {
			t.Fatalf("%s: handleKeys: %v", screenLabel(s), err)
		}
		if g.host.IsOpen(s.ID) == before {
			t.Errorf("%s: %v did not toggle the flag (still %v)", screenLabel(s), s.Key, before)
		}
		if err := g.handleKeys(press); err != nil {
			t.Fatalf("%s: handleKeys (release): %v", screenLabel(s), err)
		}
		if g.host.IsOpen(s.ID) != before {
			t.Errorf("%s: second press did not toggle back (now %v)", screenLabel(s), g.host.IsOpen(s.ID))
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
	// Test labels follow the row id.
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
