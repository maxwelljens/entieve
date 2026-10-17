package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The station services view's own tests: which buttons a station gets
// (two different service sets — absent services are HIDDEN, never
// grayed), what each button emits, and the identity line over the column
// fit.

// newServicesView wires the view over a stubbed docked station and a
// recording sink, mounted on a chrome UI so clicks run through the
// deferred queue like a real frame.
func newServicesView(t *testing.T, inner int, services []string) (*Services, *recordingSink, *chromeUI) {
	t.Helper()
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	cv := &stubView{
		docked:  true,
		station: widgets.StationInfo{Name: "Jernborg", System: "Genesis", Services: services},
	}
	sink := &recordingSink{}
	env := widgets.NewSeams(th, cv, sink)
	c := newChromeUI(env)
	s := NewServices(env, inner)
	c.AddPanel(s.Container(), s)
	c.Frame(t, 1280, 720)
	return s, sink, c
}

// TestServicesButtonsPerStation: one button per AVAILABLE service —
// pinned against two stations' service sets — plus the two buttons that
// always ride the view (Undock and Cargo). A station that lacks a
// service does not offer it at all: the button is hidden, not grayed.
func TestServicesButtonsPerStation(t *testing.T) {
	const inner = 346 // the board column's content width (game: boardInnerW)
	for _, tc := range []struct {
		name     string
		services []string
		visible  []string
		hidden   []string
	}{
		{
			name:     "haven rest: repair market refine fitting",
			services: []string{"repair", "market", "refine", "fitting"},
			visible:  []string{"undock", "cargo", "market", "fitting"},
			hidden:   []string{"industry"},
		},
		{
			name:     "outpost: market only",
			services: []string{"market"},
			visible:  []string{"undock", "cargo", "market"},
			hidden:   []string{"fitting", "industry"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newServicesView(t, inner, tc.services)
			for _, name := range tc.visible {
				b := s.Button(name)
				if b == nil {
					t.Fatalf("no %q button on the view", name)
				}
				if !s.Visible(name) || !b.GetWidget().IsVisible() {
					t.Errorf("%q button hidden at a station offering it", name)
				}
			}
			for _, name := range tc.hidden {
				b := s.Button(name)
				if b == nil {
					t.Fatalf("no %q button on the view", name)
				}
				if s.Visible(name) || b.GetWidget().IsVisible() {
					t.Errorf("%q button offered at a station lacking the service (absent = not offered)", name)
				}
				if b.GetWidget().Disabled {
					t.Errorf("%q is grayed instead of hidden (grayed implies offered)", name)
				}
			}
		})
	}
}

// TestServicesEmitsTypedIntents: every button expresses itself as a
// typed intent — Undock for the leave button, Toggle{Screen} for the
// windows (Cargo → inventory, the services → their spec windows). The
// names are the spec's own Titles, so the toggles land on rows the host
// resolves (game pins that side).
func TestServicesEmitsTypedIntents(t *testing.T) {
	s, sink, c := newServicesView(t, 346, []string{"market", "fitting", "industry", "repair", "refine"})

	for _, tc := range []struct {
		button string
		want   widgets.Intent
	}{
		{"undock", widgets.Undock{}},
		{"cargo", widgets.Toggle{Screen: "Inventory"}},
		{"market", widgets.Toggle{Screen: "Market"}},
		{"fitting", widgets.Toggle{Screen: "Fitting"}},
		{"industry", widgets.Toggle{Screen: "Factory"}},
	} {
		fireRailClick(t, c, s.Button(tc.button))
		if len(sink.got) != 1 {
			t.Fatalf("%s emitted %v, want the single %#v", tc.button, sink.got, tc.want)
		}
		if sink.got[0] != tc.want {
			t.Errorf("%s emitted %#v, want %#v", tc.button, sink.got[0], tc.want)
		}
		sink.got = nil
	}
}

// TestSvcIconsInsideCells pins the service row's icon anchoring (the
// station overview's square buttons — the same graphic-row overshoot the
// rail cells had): every glyph sits strictly inside its button square
// with equal margins ±1 px, at scale 1 and 1.5, and the square itself
// keeps its design box.
func TestSvcIconsInsideCells(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		uitest.UseScale(t, scale)
		s, _, _ := newServicesView(t, 346, []string{"market", "fitting", "industry", "repair", "refine"})
		checked := 0
		for i := range s.btns {
			b := &s.btns[i]
			if !b.visible {
				continue
			}
			cell := b.btn.GetWidget().Rect
			glyph := b.glyph.Widget().GetWidget().Rect
			if glyph.Dx() != theme.Px(theme.IconButton) || glyph.Dy() != theme.Px(theme.IconButton) {
				t.Errorf("%s: glyph box = %v, want %d² design (the Px funnel)", b.name, glyph, theme.IconButton)
			}
			if cell.Dx() != theme.Px(svcBtnW) || cell.Dy() != theme.Px(svcBtnH) {
				t.Errorf("%s: cell = %v, want %d×%d design box", b.name, cell, svcBtnW, svcBtnH)
			}
			if !glyph.In(cell) {
				t.Errorf("%s: glyph %v escapes its cell %v", b.name, glyph, cell)
				continue
			}
			l, r := glyph.Min.X-cell.Min.X, cell.Max.X-glyph.Max.X
			tp, bt := glyph.Min.Y-cell.Min.Y, cell.Max.Y-glyph.Max.Y
			if abs(l-r) > 1 || abs(tp-bt) > 1 {
				t.Errorf("%s: glyph margins L%d R%d T%d B%d in %v, want equal ±1",
					b.name, l, r, tp, bt, cell)
			}
			checked++
		}
		if checked == 0 {
			t.Fatal("no visible service buttons to check")
		}
	}
}

// TestServicesIdentityAndFit: the identity line carries the station
// name and system, and the whole row — identity over its buttons —
// measures inside the column it is hosted in, so it can never push the
// composed window past stationLayout's left column (at scale 1 and
// through the funnel).
func TestServicesIdentityAndFit(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		uitest.UseScale(t, scale)
		const inner = 346
		// Every service → five buttons: the widest the row gets.
		s, _, _ := newServicesView(t, inner, []string{"market", "fitting", "industry", "repair", "refine"})
		if want := "Jernborg · Genesis"; s.Identity() != want {
			t.Errorf("identity = %q, want %q (station name · system)", s.Identity(), want)
		}
		if s.name.Label == "" || !strings.HasPrefix("Jernborg · Genesis", s.name.Label) {
			t.Errorf("drawn label = %q, want the identity (ellipsized when tight)", s.name.Label)
		}
		w, _ := s.Container().PreferredSize()
		if w > theme.Px(inner) {
			t.Errorf("services row prefers %d px, column content is %d — the row would overflow", w, theme.Px(inner))
		}
	}
}
