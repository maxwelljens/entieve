package screens

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The Show Info body's header block: ONE row — the entity's big render on a
// dark plate, then the title the corporation mark's Sub line beside it in a
// column. The mark lives on the maker (Sub) line, never on the plate, and no
// icon sits beside the title text.

// headerRow returns the page's header row and the title/Sub column beside
// the plate. It fails the test when the header is not that one-row shape.
func headerRow(t *testing.T, w *InfoWin) (head, col *widget.Container) {
	t.Helper()
	kids := w.page.Children()
	if len(kids) == 0 {
		t.Fatal("page is empty")
	}
	head, ok := kids[0].(*widget.Container)
	if !ok {
		t.Fatalf("first page block is %T, want the header row", kids[0])
	}
	if len(head.Children()) < 2 {
		t.Fatalf("header row has %d children, want the plate + the title/Sub column", len(head.Children()))
	}
	col, ok = head.Children()[1].(*widget.Container)
	if !ok {
		t.Fatalf("header row child 1 is %T, want the title/Sub column", head.Children()[1])
	}
	return head, col
}

// TestInfoWindowHeaderHasNoIcon pins the title text: it is a bare text
// widget in the header column — no icon widget sits in the title row.
func TestInfoWindowHeaderHasNoIcon(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	kids := w.page.Children()
	if len(kids) < 2 {
		t.Fatalf("page has %d blocks, want the header row + prose (and the rest)", len(kids))
	}
	_, col := headerRow(t, w)
	title, ok := col.Children()[0].(*widget.Text)
	if !ok {
		t.Fatalf("column child 0 is %T, want the bare title text (no header icon)", col.Children()[0])
	}
	if title.Label != "Small Autocannon" {
		t.Errorf("title text = %q, want the module name", title.Label)
	}
}

// TestInfoWindowHeaderIsOneRow pins the header STRUCTURE: one row container
// holds the icon's plate and the title/Sub column side by side. The old
// layout stacked them — hero row, then title, then the Sub row.
func TestInfoWindowHeaderIsOneRow(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	head, col := headerRow(t, w)
	if n := len(head.Children()); n != 2 {
		t.Fatalf("header row has %d children, want the plate + the title/Sub column", n)
	}
	plate, ok := head.Children()[0].(*widget.Container)
	if !ok || plate != w.HeroPlate() {
		t.Fatalf("header row child 0 is %T, want the icon plate", head.Children()[0])
	}
	// The two icons share the row: the plate and the column overlap
	// vertically and the column starts to the right of the plate.
	p, c := plate.GetWidget().Rect, col.GetWidget().Rect
	if p.Min.Y >= c.Max.Y || c.Min.Y >= p.Max.Y || c.Min.X < p.Max.X {
		t.Errorf("plate %v and column %v do not sit side by side on one row", p, c)
	}
	// The Sub line — corporation mark first — is the column's second row.
	if g := w.Emblem(); g == nil || !g.GetWidget().Rect.In(c) {
		if g == nil {
			t.Error("module page lost its corporation mark")
		} else {
			t.Errorf("corporation mark %v is not in the header column %v", g.GetWidget().Rect, c)
		}
	}
}

// TestInfoWindowHeroPlateIsDark pins the entity icon's dark box: a backdrop
// widget under the icon whose fill is the palette's opaque dark role —
// distinct from the translucent window-body fill — so the icon stays
// visually separate from the name and the corporation mark.
func TestInfoWindowHeroPlateIsDark(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	plate := w.HeroPlate()
	if plate == nil {
		t.Fatal("header has no plate around the entity icon")
	}
	// ebitenui's only public "this container carries a backdrop" signal:
	// a container built with a background image tracks hover (container.go).
	if !plate.GetWidget().TrackHover {
		t.Fatal("the plate draws no backdrop: the icon is not boxed")
	}
	// The fill is the palette's opaque dark, distinct from the window-body
	// fill it lies on (theme.WellBox draws it; TestWellBoxIsTheDarkPlate
	// pins the box's colours and radius).
	pal := env.Pal()
	if got := theme.WellFill(pal); got == theme.WindowBody(pal.BG) {
		t.Errorf("plate fill %v is the window-body fill, want a distinct dark role", got)
	}
	// The unattributed page (ore: no maker) is boxed the same way.
	if !w.Show(widgets.ItemRef("ore-sterngrit")) {
		t.Fatal("ore did not resolve")
	}
	if w.HeroPlate() == nil || !w.HeroPlate().GetWidget().TrackHover {
		t.Error("unattributed page lost its icon box")
	}
}

// TestInfoWindowHeroPlatePadsTheIcon pins the plate's bbox proof: the box
// strictly contains the icon's rect with a visible pad on every side (the
// icon keeps its 128 px token; the plate grows around it).
func TestInfoWindowHeroPlatePadsTheIcon(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	plate, icon := w.HeroPlate(), w.Hero()
	if plate == nil || icon == nil {
		t.Fatal("header has no plate or no icon")
	}
	box, ir := plate.GetWidget().Rect, icon.GetWidget().Rect
	if !ir.In(box) {
		t.Fatalf("icon %v is not inside its plate %v", ir, box)
	}
	pad := theme.Px(8)
	if ir.Min.X-box.Min.X < pad || ir.Min.Y-box.Min.Y < pad ||
		box.Max.X-ir.Max.X < pad || box.Max.Y-ir.Max.Y < pad {
		t.Errorf("plate %v pads the icon %v by less than %d px on a side", box, ir, pad)
	}
	if got := ir.Dx(); got != theme.Px(theme.IconHero) {
		t.Errorf("icon box = %d px, want the hero token %d (the plate must not shrink it)",
			got, theme.Px(theme.IconHero))
	}
}

// TestInfoWindowHeroBlock pins the hero: the entity's icon at the hero token
// alone on its plate — a panel must not show the corporation mark twice (the
// maker line carries it), and the header row exists without a mark on the
// unattributed pages too.
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
	if b := h.Image.Bounds(); b.Dx() != 128 {
		t.Errorf("hero box = %v, want the 128 px info hero", b)
	}
	plate := w.HeroPlate()
	if plate == nil {
		t.Fatal("hero has no plate")
	}
	if n := len(plate.Children()); n != 1 {
		t.Errorf("plate has %d children, want the entity icon alone", n)
	} else if plate.Children()[0] != h {
		t.Error("the plate does not hold the hero graphic")
	}

	// An unattributed def draws the same plate + hero (an ore has no maker).
	if !w.Show(widgets.ItemRef("ore-sterngrit")) {
		t.Fatal("ore did not resolve")
	}
	if w.Hero() == nil || w.HeroPlate() == nil {
		t.Error("ore page lost its boxed hero")
	}
	if w.Emblem() != nil {
		t.Error("ore page drew a corporation mark")
	}
}

// TestInfoWindowMakerLineCarriesCorpArt pins the Sub line's mark: the dropped
// corporation art (theme.CorpIconFor) at the info corp token (80 px — twice
// the fitting window's block), not the vector emblem — the corp icons replace
// the emblems on the info maker line. The mark moved into the header column;
// its size and value are unchanged.
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
	if b := g.Image.Bounds(); b.Dx() != 80 {
		t.Errorf("maker-line mark box = %v, want the 80 px info corp mark", b)
	}
	if b := g.Image.Bounds(); b.Dx() != theme.Px(theme.IconCorpInfo) {
		t.Errorf("maker-line mark box = %v, want %d px (theme.IconCorpInfo)", b, theme.Px(theme.IconCorpInfo))
	}
	if g.Image != icons.Get(wantID, theme.Px(theme.IconCorpInfo), env.Pal().Text) {
		t.Errorf("maker-line mark is not the drop-in corp art %q", wantID)
	}
	// The mark rides the Sub line under the title, inside the header column
	// beside the plate.
	_, col := headerRow(t, w)
	if !g.GetWidget().Rect.In(col.GetWidget().Rect) {
		t.Errorf("mark %v is not in the header column %v", g.GetWidget().Rect, col.GetWidget().Rect)
	}
	if sub := col.Children()[1].(*widget.Container); !g.GetWidget().Rect.In(sub.GetWidget().Rect) {
		t.Errorf("mark %v is not on the Sub line %v", g.GetWidget().Rect, sub.GetWidget().Rect)
	}
}
