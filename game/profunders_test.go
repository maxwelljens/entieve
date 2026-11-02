package game

import (
	"image"
	"testing"

	"github.com/entieve/entieve/uikit/widgets"
)

// TestProficiencyOverviewFollowsTheTree pins the pair's lifecycle: the
// overview window opens with the tree, its × closes only itself (the tree
// keeps running), and closing the tree closes both. Opening the tree again
// brings the overview back.
func TestProficiencyOverviewFollowsTheTree(t *testing.T) {
	g := newShowcaseGame(t)
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idProficiencyOverview) {
		t.Fatal("the overview is open before the tree ever was")
	}

	g.host.SetOpen(idProficiency, true)
	pump(t, g, 1280, 720)
	if !g.host.IsOpen(idProficiencyOverview) || !g.host.Mounted(idProficiencyOverview) {
		t.Fatal("opening the tree did not open the overview window")
	}

	// The overview's own × (the row's close) keeps it shut for this visit.
	g.toggleScreenID(idProficiencyOverview)
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idProficiencyOverview) {
		t.Fatal("the overview reopened itself after its × close")
	}
	if !g.host.IsOpen(idProficiency) {
		t.Fatal("closing the overview took the tree with it")
	}

	// Reopening the tree reopens the overview.
	g.host.SetOpen(idProficiency, false)
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idProficiencyOverview) {
		t.Fatal("the overview outlived the tree")
	}
	g.host.SetOpen(idProficiency, true)
	pump(t, g, 1280, 720)
	if !g.host.IsOpen(idProficiencyOverview) {
		t.Fatal("the overview did not come back with the tree")
	}

	// Esc: the tree and its overview go together.
	if err := g.onEscape(); err != nil {
		t.Fatalf("onEscape: %v", err)
	}
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idProficiency) || g.host.IsOpen(idProficiencyOverview) {
		t.Fatal("Esc left the tree or its overview open")
	}
}

// TestProficiencyCardEscBacksOutFirst: with the family card up, Esc closes
// the card and leaves the tree; a second Esc closes the tree.
func TestProficiencyCardEscBacksOutFirst(t *testing.T) {
	g := newShowcaseGame(t)
	pump(t, g, 1280, 720)
	g.host.SetOpen(idProficiency, true)
	pump(t, g, 1280, 720)

	g.kit.OpenProfFamily(widgets.ShowProfFamily{Family: "mining", At: image.Pt(640, 360), Level: 1})
	if !g.kit.ProfFamilyOpen() {
		t.Fatal("fixture: the family card did not open")
	}
	if err := g.onEscape(); err != nil {
		t.Fatalf("onEscape: %v", err)
	}
	if g.kit.ProfFamilyOpen() {
		t.Fatal("Esc did not close the family card")
	}
	if !g.host.IsOpen(idProficiency) {
		t.Fatal("Esc closed the tree along with the card")
	}
}
