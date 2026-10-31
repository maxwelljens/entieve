package game

import (
	"fmt"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The contract board is a Screen panel (uikit/screens/stationboard.go):
// these helpers reach it through the host for the game-composition pins —
// the window's column, the session's menu, and the intent→command tail.
// The board's own model, rows and geometry are pinned in the panel's
// package.

// dockedShowcaseGame builds the showcase session, docks at the current
// system's station and pumps one frame so the contract board is laid out.
func dockedShowcaseGame(t *testing.T, w, h int, ptr ...*widgets.Pointer) *Game {
	t.Helper()
	g := newShowcaseGame(t, ptr...)
	g.applySize(w, h)
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, w, h)
	return g
}

// click fires the widget-level left click ebitenui would fire from input,
// then flushes the deferred event queue through a kit update.
func click(t *testing.T, g *Game, btn *widget.Button) {
	t.Helper()
	w := btn.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: w,
		Button: ebiten.MouseButtonLeft,
	})
	if err := g.kit.Update(); err != nil {
		t.Fatalf("kit.Update: %v", err)
	}
}

// stationBoard returns the docked station's mounted board panel.
func stationBoard(t *testing.T, g *Game) *uikit.ContactsPanel {
	t.Helper()
	b := bodyOf[*uikit.ContactsPanel](g.host, idServices)
	if b == nil {
		t.Fatal("station board panel missing")
	}
	return b
}

// cappedMissions builds n accepted work rows (distinct ids, materialized
// defs) to drive the board's active-section caps without running combat.
func cappedMissions(n int) []sim.MissionState {
	out := make([]sim.MissionState, 0, n)
	for i := 0; i < n; i++ {
		id := content.ContractID(fmt.Sprintf("offer-test-%d", i))
		def := &content.ContractDef{
			ID: id, Title: fmt.Sprintf("Work %d", i), Kind: content.ContractMine,
			Station: "cel-haven-rest", Level: 1,
			Objective: content.ContractObjective{Item: "ore-cordite", Count: 10},
			Reward:    content.ContractReward{Credits: 1000, StandingGain: 0.1},
		}
		out = append(out, sim.MissionState{ID: id, Def: def})
	}
	return out
}

// findRow returns the board row with the given offer id.
func findRow(t *testing.T, rows []uikit.ContactRow, id content.ContractID) uikit.ContactRow {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no board row for %s", id)
	return uikit.ContactRow{}
}

// setMissions swaps the sim's accepted contracts (save surgery) so tests
// can drive progress and row caps without running combat.
func setMissions(t *testing.T, g *Game, ms ...sim.MissionState) {
	t.Helper()
	sv, err := g.s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	sv.Missions = ms
	if err := g.s.Restore(sv); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}

// TestStationColumnClearsLog: at both design sizes, with the worst-case
// board (every offering accepted, so both sections sit at their caps), the
// station column stays the fitted column and its bottom stays above the
// log's pocket — the board's row caps exist to keep it there.
func TestStationColumnClearsLog(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := dockedShowcaseGame(t, size[0], size[1])
		setMissions(t, g, cappedMissions(5)...)
		g.syncMounts()
		pump(t, g, size[0], size[1])

		station := g.host.Window(idServices).GetContainer().GetWidget().Rect
		log := bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect
		if station.Min.X < 0 || station.Max.X > size[0] || station.Max.Y > size[1] {
			t.Errorf("%dx%d: station window %v out of bounds", size[0], size[1], station)
		}
		if want := stationRect(station.Dy()); station != want {
			t.Errorf("%dx%d: station window %v, want the fitted column %v", size[0], size[1], station, want)
		}
		if station.Max.Y > log.Min.Y {
			t.Errorf("%dx%d: station window %v overlaps the log %v",
				size[0], size[1], station, log)
		}
	}
}
