package screens

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The Show Info body's hero block: the entity's big render at the top of the
// body, alone — the corporation mark lives on the maker (Sub) line — and no
// icon in the title row (the header icon this window used to draw is gone).

// TestInfoWindowHeaderHasNoIcon pins the header anatomy: the page's first
// block is the hero, the row under it is the bare title text — no icon
// widget sits in the title row any more.
func TestInfoWindowHeaderHasNoIcon(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	kids := w.page.Children()
	if len(kids) < 2 {
		t.Fatalf("page has %d blocks, want hero + title (and the rest)", len(kids))
	}
	if _, ok := kids[0].(*widget.Container); !ok {
		t.Errorf("first page block is %T, want the hero row", kids[0])
	}
	title, ok := kids[1].(*widget.Text)
	if !ok {
		t.Fatalf("title row is %T, want the bare title text (no header icon)", kids[1])
	}
	if title.Label != "Small Autocannon" {
		t.Errorf("title text = %q, want the module name", title.Label)
	}
}

// TestInfoWindowHeroBlock pins the hero: the entity's icon at the hero token
// and nothing else beside it — a panel must not show the corporation mark
// twice (the maker line carries it).
func TestInfoWindowHeroBlock(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	h := w.Hero()
	if h == nil {
		t.Fatal("module page has no hero graphic")
	}
	if b := h.Image.Bounds(); b.Dx() != theme.Px(theme.IconHero) {
		t.Errorf("hero box = %v, want %d px", b, theme.Px(theme.IconHero))
	}
	hero, ok := w.page.Children()[0].(*widget.Container)
	if !ok {
		t.Fatalf("first page block is %T, want the hero row", w.page.Children()[0])
	}
	if n := len(hero.Children()); n != 1 {
		t.Errorf("hero row has %d children, want the entity icon alone", n)
	}

	// An unattributed def draws the same hero (an ore has no maker).
	if !w.Show(widgets.ItemRef("ore-sterngrit")) {
		t.Fatal("ore did not resolve")
	}
	if w.Hero() == nil {
		t.Error("ore page lost its hero")
	}
	if w.Emblem() != nil {
		t.Error("ore page drew a corporation mark")
	}
}

// TestInfoWindowMakerLineCarriesCorpArt pins the Sub line's mark: the dropped
// corporation art (theme.CorpIconFor) at the corp token, not the vector
// emblem — the corp icons replace the emblems on the info maker line.
func TestInfoWindowMakerLineCarriesCorpArt(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	ref := widgets.ModuleRef("mod-autocannon-s")
	env.ShowInfo(ref)
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	page, ok := infoPageFor(env, ref)
	if !ok {
		t.Fatal("fixture page did not resolve")
	}
	g := w.Emblem()
	if g == nil || g.Image == nil {
		t.Fatal("manufactured module page has no maker-line mark")
	}
	wantID := theme.CorpIconFor(page.Emblem)
	if wantID == "" {
		t.Fatalf("%s has no drop-in corp art", page.Emblem)
	}
	if b := g.Image.Bounds(); b.Dx() != theme.Px(theme.IconCorp) {
		t.Errorf("maker-line mark box = %v, want %d px", b, theme.Px(theme.IconCorp))
	}
	if g.Image != icons.Get(wantID, theme.Px(theme.IconCorp), env.Pal().Text) {
		t.Errorf("maker-line mark is not the drop-in corp art %q", wantID)
	}
}
