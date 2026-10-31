package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The station services row's own tests: what it carries (the docked-at
// identity and Undock — every window is the rail's row, so the strip offers
// no second door to one), what its button emits, and the identity line over
// the column fit.

// newServicesView wires the view over a stubbed docked station and a
// recording sink, mounted on a chrome UI so clicks run through the
// deferred queue like a real frame.
func newServicesView(t *testing.T, inner int, services []string) (*Services, *recordingSink, *chromeUI) {
	t.Helper()
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	cv := &stubView{
		docked:  true,
		station: widgets.StationInfo{Name: "Jalvrek", System: "Genesis", Services: services},
	}
	sink := &recordingSink{}
	env := widgets.NewSeams(th, cv, sink, nil)
	c := newChromeUI(env)
	s := NewServices(env, inner)
	c.AddPanel(s.Container(), s)
	c.Frame(t, 1280, 720)
	return s, sink, c
}

// TestServicesRowCarriesNoWindowButtons: the strip is the station's face and
// its leave verb. Every window a station offers is already a rail row
// (docs/GAMEPLAY.md, Contacts & standing), so neither a service's window
// button nor a Cargo/Inventory shortcut is drawn at any station's service
// set.
func TestServicesRowCarriesNoWindowButtons(t *testing.T) {
	const inner = 346 // the board column's content width (game: boardInnerW)
	for _, tc := range []struct {
		name     string
		services []string
	}{
		{"haven rest: repair market refine fitting", []string{"repair", "market", "refine", "fitting"}},
		{"outpost: market only", []string{"market"}},
		{"bare: repair only", []string{"repair"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newServicesView(t, inner, tc.services)
			b := s.Button("undock")
			if b == nil {
				t.Fatal("no Undock button on the view")
			}
			if !s.Visible("undock") || !b.GetWidget().IsVisible() {
				t.Error("Undock button hidden while docked")
			}
			// Every window opener the strip used to carry: gone.
			for _, name := range []string{"cargo", "market", "fitting", "industry", "refine"} {
				if s.Button(name) != nil {
					t.Errorf("%q button still on the strip — the rail owns that window", name)
				}
			}
			for _, svc := range content.Services {
				if s.Button(svc.ID) != nil {
					t.Errorf("%q service button still on the strip — a second door to %q", svc.ID, svc.Window)
				}
			}
			// The row is the identity and the one cell, nothing else.
			if got, want := len(s.Container().Children()), 2; got != want {
				t.Errorf("row children = %d, want identity + Undock (%d)", got, want)
			}
		})
	}
}

// TestServicesEmitsUndock: the one button expresses itself as the Undock
// intent (the sim's CmdUndock), and nothing else on the row is pressable.
func TestServicesEmitsUndock(t *testing.T) {
	s, sink, c := newServicesView(t, 346, []string{"market", "fitting", "industry", "repair", "refine"})
	fireRailClick(t, c, s.Button("undock"))
	if len(sink.got) != 1 {
		t.Fatalf("undock emitted %v, want the single Undock", sink.got)
	}
	if _, ok := sink.got[0].(widgets.Undock); !ok {
		t.Errorf("undock emitted %#v, want widgets.Undock", sink.got[0])
	}
}

// TestSvcIconsInsideCells pins the service row's icon anchoring (the
// station overview's square buttons — the same graphic-row overshoot the
// rail cells had): the glyph sits strictly inside its button square with
// equal margins ±1 px, at scale 1 and 1.5, and the square itself keeps its
// design box.
func TestSvcIconsInsideCells(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		uitest.UseScale(t, scale)
		s, _, _ := newServicesView(t, 346, []string{"market", "fitting", "industry", "repair", "refine"})
		checked := 0
		for i := range s.btns {
			b := &s.btns[i]
			if !b.btn.GetWidget().IsVisible() {
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

// TestServicesIdentityAndFit: the identity line carries the station name and
// system, and the whole row — identity over its button — measures inside the
// column it is hosted in, so it can never push the services window past its
// station column (at scale 1 and through the funnel).
func TestServicesIdentityAndFit(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		uitest.UseScale(t, scale)
		const inner = 346
		s, _, _ := newServicesView(t, inner, []string{"market", "fitting", "industry", "repair", "refine"})
		const want = "Jalvrek · Genesis"
		if s.Identity() != want {
			t.Errorf("identity = %q, want %q (station name · system)", s.Identity(), want)
		}
		if s.name.Label == "" || !strings.HasPrefix(want, strings.TrimSuffix(s.name.Label, "…")) {
			t.Errorf("drawn label = %q, want the identity (ellipsized when tight)", s.name.Label)
		}
		w, _ := s.Container().PreferredSize()
		if w > theme.Px(inner) {
			t.Errorf("services row prefers %d px, column content is %d — the row would overflow", w, theme.Px(inner))
		}
	}
}
