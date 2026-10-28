package screens

import (
	"fmt"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// The rendered board geometry, moved with the panel: the rows are the
// table module's rich rows and these are the rects the bespoke board laid
// out. The panel reads the production view over a real sim here (the
// seam-purity pins live in stationboard_test.go); no game and no GPU.

// pinRect asserts one widget's laid-out rect.
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

// dockedBoard builds the panel over the showcase sim docked at cel and
// lays it out once at the declared column.
func dockedBoard(t *testing.T, cel string) (*StationBoard, *testEnv, *ebitenui.UI) {
	t.Helper()
	env, _ := newTestEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: content.CelID(cel)})
	b := NewStationBoard(env)
	ui := newWindowUI(t, env, b.Win())
	uitest.PumpWindow(t, ui, b.Sync, 1280, 720)
	return b, env, ui
}

// TestBoardOnTableRowRects pins the board's rendered geometry at the
// default size (1280×720, scale 1): the section rects, every row rect and
// the line/icon/button rects inside each row shape — available (3-line
// stack), active (title + Abandon), the "+N more"/empty-state note (one
// line, centred in the stretched strip) and a standing-gated row (the
// requirement beside the summary).
//
// The left column carries the navigation rail's gutter (RailInset = 87):
// every LEFT edge sits 87 px right of the pre-rail pin, every
// right-anchored edge (row right edge, buttons, the 395 line cap) is
// unchanged — the column's right edge never moved, so the composition to
// its right is untouched. Widths of content-sized rects are identical.
func TestBoardOnTableRowRects(t *testing.T) {
	// --- The default showcase board: three available rows over the
	// empty-state note.
	b, env, ui := dockedBoard(t, "cel-haven-rest")
	pinRect(t, "available section", b.avail, image.Rect(103, 85, 398, 327))
	wantRows := []image.Rectangle{
		image.Rect(103, 85, 398, 163),
		image.Rect(103, 167, 398, 245),
		image.Rect(103, 249, 398, 327),
	}
	avail := b.avail.Children()
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
	pinRect(t, "title line", lineAt(t, first, 0), image.Rect(106, 85, 395, 125))
	pinRect(t, "summary line", lineAt(t, first, 1), image.Rect(106, 125, 228, 144))
	pinRect(t, "reward line", lineAt(t, first, 2), image.Rect(106, 144, 395, 163))
	line1 := lineAt(t, first, 0).(*widget.Container)
	pinRect(t, "icon+title cell", line1.Children()[0], image.Rect(106, 94, 297, 115))
	pinRect(t, "accept button", line1.Children()[1], image.Rect(303, 85, 395, 125))
	lead := line1.Children()[0].(*widget.Container)
	pinRect(t, "kind icon", lead.Children()[0], image.Rect(106, 96, 122, 112))
	pinRect(t, "title", lead.Children()[1], image.Rect(126, 94, 297, 115))

	pinRect(t, "active section", b.active, image.Rect(103, 358, 398, 377))
	note := b.active.Children()[0].(*widget.Container)
	pinRect(t, "empty-state row", note, image.Rect(103, 358, 398, 377))
	pinRect(t, "empty-state line", lineAt(t, note, 0), image.Rect(106, 358, 231, 377))

	// --- Row caps: an active contract beside a "+N more" note, both
	// stretched to the section's shared row height — the note's line
	// centres in its strip instead of top-flushing.
	b, env, ui = dockedBoard(t, "cel-haven-rest")
	setSimMissions(t, env.Sim,
		sim.MissionState{ID: "contract-solace-cleanup-1"},
		sim.MissionState{ID: "contract-solace-cordite-1"},
		sim.MissionState{ID: "contract-solace-supply-run-1"},
		sim.MissionState{ID: "contract-penumbra-den-hunt-1"},
		sim.MissionState{ID: "contract-penumbra-lucidite-1"},
	)
	uitest.PumpWindow(t, ui, b.Sync, 1280, 720)
	pinRect(t, "capped active section", b.active, image.Rect(103, 358, 398, 442))
	active := b.active.Children()
	if len(active) != 2 {
		t.Fatalf("capped active rows = %d, want 2", len(active))
	}
	pinRect(t, "active contract row", active[0], image.Rect(103, 358, 398, 398))
	actLine := lineAt(t, active[0].(*widget.Container), 0)
	pinRect(t, "active title line", actLine, image.Rect(106, 358, 395, 398))
	pinRect(t, "abandon button", actLine.(*widget.Container).Children()[1], image.Rect(303, 358, 395, 398))
	pinRect(t, "capped note row", active[1], image.Rect(103, 402, 398, 442))
	pinRect(t, "centred note line", lineAt(t, active[1].(*widget.Container), 0), image.Rect(106, 412, 159, 431))

	// All five missions are taken, so every offering wears the model's
	// own button label and gate: "Accepted", grayed (never hidden).
	if got := len(b.Rows(false)); got != availMaxRows {
		t.Fatalf("capped available rows = %d, want %d", got, availMaxRows)
	}
	for _, r := range b.Rows(false) {
		if got := r.Btn.Text().Label; got != "Accepted" {
			t.Errorf("%s button label = %q, want Accepted", r.ID, got)
		}
		if !r.Btn.GetWidget().Disabled {
			t.Errorf("%s: accepted row's button is enabled", r.ID)
		}
	}
	// The documented split survives: the context menu still offers the
	// verb (GAMEPLAY: "contract board rows — Accept / Abandon (grayed
	// when gated)"), grayed at the same gate as the marker button.
	fireRightClick(t, ui, b.Rows(false)[0].Root.GetWidget())
	if e := entry(t, env, "Accept"); !e.Disabled {
		t.Error("accepted row's menu Accept entry is enabled")
	}
	if n := len(env.Menu().Entries()); n != 1 {
		t.Errorf("accepted row menu = %d entries, want the single Accept", n)
	}
	env.Menu().Close()

	// --- The gated board (Hauvnok): the standing requirement rides
	// line 2 beside the summary, at the row's full content width.
	b, _, ui = dockedBoard(t, "cel-penumbra-port")
	pinRect(t, "gated section", b.avail, image.Rect(103, 85, 398, 245))
	gated := b.avail.Children()[0].(*widget.Container)
	pinRect(t, "gated row", gated, image.Rect(103, 85, 398, 163))
	gatedLine2 := lineAt(t, gated, 1).(*widget.Container)
	pinRect(t, "gated summary line", gatedLine2, image.Rect(106, 125, 391, 144))
	pinRect(t, "gated summary", gatedLine2.Children()[0], image.Rect(106, 125, 239, 144))
	pinRect(t, "gated requirement", gatedLine2.Children()[1], image.Rect(245, 125, 391, 144))
}

// fireRightClick fires a widget-level right-button click (with the widget
// and an offset, so the menu anchors where the cursor was) and flushes the
// deferred queue.
func fireRightClick(t *testing.T, ui *ebitenui.UI, w *widget.Widget) {
	t.Helper()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 9,
		OffsetY: 5,
	})
	ui.Update()
}

// setSimMissions swaps the sim's accepted contracts (save surgery).
func setSimMissions(t *testing.T, s *sim.Sim, ms ...sim.MissionState) {
	t.Helper()
	sv, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	sv.Missions = ms
	if err := s.Restore(sv); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}
