package screens

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The Show Info body's hero block: the entity's big render at the top of the
// body with the corporation logo beside it, and no icon in the title row
// (the header icon this window used to draw is gone).

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
// and, for a manufactured def, the corporation's dropped logo beside it at
// the corp token — the hero strictly bigger than the logo.
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
	logo := w.CorpLogo()
	if logo == nil {
		t.Fatal("manufactured module page has no corporation logo")
	}
	lb := logo.Image.Bounds()
	if lb.Dx() != theme.Px(theme.IconCorp) {
		t.Errorf("corp logo box = %v, want %d px", lb, theme.Px(theme.IconCorp))
	}
	if lb.Dx() >= h.Image.Bounds().Dx() {
		t.Errorf("corp logo %d px is not smaller than the hero %d px", lb.Dx(), h.Image.Bounds().Dx())
	}

	// Unattributed defs draw the hero alone (no logo): an ore has no maker.
	if !w.Show(widgets.ItemRef("ore-sterngrit")) {
		t.Fatal("ore did not resolve")
	}
	if w.Hero() == nil {
		t.Error("ore page lost its hero")
	}
	if w.CorpLogo() != nil {
		t.Error("ore page drew a corporation logo")
	}
}
