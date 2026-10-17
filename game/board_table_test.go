package game

import (
	"fmt"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
)

// pinRect asserts one widget's laid-out rect (the board-on-table render
// pin: the rows are table-module rich rows now, and their geometry is the
// look the bespoke board had).
func pinRect(t *testing.T, name string, w widget.PreferredSizeLocateableWidget, want image.Rectangle) {
	t.Helper()
	if got := w.GetWidget().Rect; got != want {
		t.Errorf("%s rect = %v, want %v", name, got, want)
	}
}

// lineAt returns row root's i-th line widget (the rich row's children ARE
// its lines).
func lineAt(t *testing.T, root *widget.Container, i int) widget.PreferredSizeLocateableWidget {
	t.Helper()
	kids := root.Children()
	if i >= len(kids) {
		t.Fatalf("row has %d lines, want line %d", len(kids), i)
	}
	return kids[i]
}

// TestBoardOnTableRowRects pins the board's rendered geometry at the
// default size (1280×720, scale 1): the section rects, every row rect and
// the line/icon/button rects inside each row shape — available (3-line
// stack), active (title + Abandon), the "+N more"/empty-state note (one
// line, centred in the stretched strip) and a standing-gated row (the
// requirement beside the summary). The board renders through the table
// module's rich rows; these are the rects the bespoke board laid out.
//
// The left column carries the navigation rail's gutter (uikit.RailInset
// = 63): every LEFT edge sits 63 px right of the pre-rail pin, every
// right-anchored edge (row right edge, buttons, the 395 line cap) is
// unchanged — the column's right edge never moved, so the composition to
// its right is untouched. Widths of content-sized rects are identical.
func TestBoardOnTableRowRects(t *testing.T) {
	// --- The default showcase board: three available rows over the
	// empty-state note.
	g := dockedShowcaseGame(t, 1280, 720)
	pinRect(t, "available section", g.stAvail, image.Rect(79, 85, 398, 327))
	wantRows := []image.Rectangle{
		image.Rect(79, 85, 398, 163),
		image.Rect(79, 167, 398, 245),
		image.Rect(79, 249, 398, 327),
	}
	avail := g.stAvail.Children()
	if len(avail) != len(wantRows) {
		t.Fatalf("available rows = %d, want %d", len(avail), len(wantRows))
	}
	for i, r := range avail {
		pinRect(t, fmt.Sprintf("available row %d", i), r, wantRows[i])
	}
	first := avail[0].(*widget.Container)
	if len(first.Children()) != 3 {
		t.Fatalf("available row lines = %d, want 3", len(first.Children()))
	}
	pinRect(t, "title line", lineAt(t, first, 0), image.Rect(82, 85, 395, 125))
	pinRect(t, "summary line", lineAt(t, first, 1), image.Rect(82, 125, 204, 144))
	pinRect(t, "reward line", lineAt(t, first, 2), image.Rect(82, 144, 395, 163))
	line1 := lineAt(t, first, 0).(*widget.Container)
	pinRect(t, "icon+title cell", line1.Children()[0], image.Rect(82, 94, 297, 115))
	pinRect(t, "accept button", line1.Children()[1], image.Rect(303, 85, 395, 125))
	lead := line1.Children()[0].(*widget.Container)
	pinRect(t, "kind icon", lead.Children()[0], image.Rect(82, 96, 98, 112))
	pinRect(t, "title", lead.Children()[1], image.Rect(102, 94, 297, 115))

	pinRect(t, "active section", g.stActive, image.Rect(79, 358, 398, 377))
	note := g.stActive.Children()[0].(*widget.Container)
	pinRect(t, "empty-state row", note, image.Rect(79, 358, 398, 377))
	pinRect(t, "empty-state line", lineAt(t, note, 0), image.Rect(82, 358, 207, 377))

	// --- Row caps: an active contract beside a "+N more" note, both
	// stretched to the section's shared row height — the note's line
	// centres in its strip instead of top-flushing.
	g = dockedShowcaseGame(t, 1280, 720)
	setMissions(t, g,
		sim.MissionState{ID: "contract-solace-cleanup-1"},
		sim.MissionState{ID: "contract-solace-cordite-1"},
		sim.MissionState{ID: "contract-solace-supply-run-1"},
		sim.MissionState{ID: "contract-penumbra-den-hunt-1"},
		sim.MissionState{ID: "contract-penumbra-lucidite-1"},
	)
	g.syncMounts()
	pump(t, g, 1280, 720)
	pinRect(t, "capped active section", g.stActive, image.Rect(79, 358, 398, 442))
	active := g.stActive.Children()
	if len(active) != 2 {
		t.Fatalf("capped active rows = %d, want 2", len(active))
	}
	pinRect(t, "active contract row", active[0], image.Rect(79, 358, 398, 398))
	actLine := lineAt(t, active[0].(*widget.Container), 0)
	pinRect(t, "active title line", actLine, image.Rect(82, 358, 395, 398))
	pinRect(t, "abandon button", actLine.(*widget.Container).Children()[1], image.Rect(303, 358, 395, 398))
	pinRect(t, "capped note row", active[1], image.Rect(79, 402, 398, 442))
	pinRect(t, "centred note line", lineAt(t, active[1].(*widget.Container), 0), image.Rect(82, 412, 135, 431))

	// All five missions are taken, so every offering wears the model's
	// own button label and gate: "Accepted", grayed (never hidden).
	if len(g.stAvailRows) != availMaxRows {
		t.Fatalf("capped available rows = %d, want %d", len(g.stAvailRows), availMaxRows)
	}
	for _, r := range g.stAvailRows {
		if got := r.btn.Text().Label; got != "Accepted" {
			t.Errorf("%s button label = %q, want Accepted", r.id, got)
		}
		if !r.btn.GetWidget().Disabled {
			t.Errorf("%s: accepted row's button is enabled", r.id)
		}
	}
	// The documented split survives the port: the context menu still
	// offers the verb (GAMEPLAY: "contract board rows — Accept / Abandon
	// (grayed when gated)"), grayed at the same gate as the marker button.
	rightClick(t, g, g.stAvailRows[0].root.GetWidget())
	if e := menuEntry(t, g, "Accept"); !e.Disabled {
		t.Error("accepted row's menu Accept entry is enabled")
	}
	if n := len(g.kit.Menu().Entries()); n != 1 {
		t.Errorf("accepted row menu = %d entries, want the single Accept", n)
	}
	g.kit.Menu().Close()

	// --- The gated board (Przystań): the standing requirement rides
	// line 2 beside the summary, at the row's full content width.
	g = dockedShowcaseGame(t, 1280, 720)
	g.s.Docked.Set(g.s.Player(), sim.Docked{At: "cel-penumbra-port"})
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	pinRect(t, "gated section", g.stAvail, image.Rect(79, 85, 398, 245))
	gated := g.stAvail.Children()[0].(*widget.Container)
	pinRect(t, "gated row", gated, image.Rect(79, 85, 398, 163))
	gatedLine2 := lineAt(t, gated, 1).(*widget.Container)
	pinRect(t, "gated summary line", gatedLine2, image.Rect(82, 125, 367, 144))
	pinRect(t, "gated summary", gatedLine2.Children()[0], image.Rect(82, 125, 215, 144))
	pinRect(t, "gated requirement", gatedLine2.Children()[1], image.Rect(221, 125, 367, 144))
}
