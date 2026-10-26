package game

import (
	"image"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// dockedShowcaseGame builds the showcase session, docks at the current
// system's station and pumps one frame so the contract board is laid out.
func dockedShowcaseGame(t *testing.T, w, h int) *Game {
	t.Helper()
	g := newShowcaseGame(t)
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

// findRow returns the board row with the given contract id.
func findRow(t *testing.T, rows []stationRow, id content.ContractID) stationRow {
	t.Helper()
	for _, r := range rows {
		if r.id == id {
			return r
		}
	}
	t.Fatalf("no board row for %s", id)
	return stationRow{}
}

// rowTitleText digs the title text out of a row's first line: note rows
// hold a bare text, contract rows hold a line container whose first cell is
// either the text directly or the [icon, text] cell.
func rowTitleText(w widget.PreferredSizeLocateableWidget) *widget.Text {
	switch v := w.(type) {
	case *widget.Text:
		return v
	case *widget.Container:
		for _, k := range v.Children() {
			if t := rowTitleText(k); t != nil {
				return t
			}
		}
	}
	return nil
}

// availLine returns one text line of an available row: 0 = title,
// 1 = objective summary, 2 = reward.
func availLine(t *testing.T, g *Game, id content.ContractID, line int) string {
	t.Helper()
	var row *widget.Container
	for _, r := range g.stAvailRows {
		if r.id == id {
			row = r.root
			break
		}
	}
	if row == nil {
		t.Fatalf("no available row for %s", id)
	}
	switch line {
	case 0:
		title := rowTitleText(row.Children()[0])
		if title == nil {
			t.Fatalf("%s: no title text in the row", id)
		}
		return title.Label
	case 1:
		// Objective summary: a bare text on open rows, a
		// summary|requirement container on gated ones.
		if ctr, ok := row.Children()[1].(*widget.Container); ok {
			return ctr.Children()[0].(*widget.Text).Label
		}
		return row.Children()[1].(*widget.Text).Label
	default:
		return row.Children()[2].(*widget.Text).Label
	}
}

// activeTitle returns the first line of the i-th active row ("Title —
// progress", or the empty-state / "+N more" note).
func activeTitle(t *testing.T, g *Game, i int) string {
	t.Helper()
	kids := g.stActive.Children()
	if i >= len(kids) {
		t.Fatalf("only %d active rows", len(kids))
	}
	// Contract rows carry a line container (its first cell is the title
	// text, behind the kind icon); note rows hold the text directly.
	title := rowTitleText(kids[i])
	if title == nil {
		t.Fatalf("active row %d: no title text", i)
	}
	return title.Label
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

// ---------------------------------------------------------------------------
// Pure row layout — the pin
// ---------------------------------------------------------------------------

// TestBoardRowRects pins the pure board row layout: exact rects for a
// known inner, then the invariants (equal heights, equal gaps, pairwise
// non-overlap, everything inside the inner rect) on the real section
// rects at1280×720 and 1024×700 plus the window minimum.
func TestBoardRowRects(t *testing.T) {
	// Exact: 176 px of inner, 3 rows, gap 4 → 56 each.
	inner := image.Rect(0, 0, boardInnerW, 176)
	want := []image.Rectangle{
		image.Rect(0, 0, boardInnerW, 56),
		image.Rect(0, 60, boardInnerW, 116),
		image.Rect(0, 120, boardInnerW, 176),
	}
	got := boardRowRects(inner, 3)
	if len(got) != len(want) {
		t.Fatalf("boardRowRects(_, 3) = %d rects, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
	if boardRowRects(inner, 0) != nil {
		t.Error("n = 0 must produce no rows")
	}
	if rows := boardRowRects(image.Rect(0, 0, boardInnerW, 3), 5); rows != nil {
		t.Errorf("degenerate inner produced %d rows, want nil", len(rows))
	}

	// Invariants at both design sizes and at the window minimum.
	inners := map[string]image.Rectangle{}
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := dockedShowcaseGame(t, size[0], size[1])
		inners[sectionName(size)] = g.stAvail.GetWidget().Rect
	}
	inners["window minimum"] = image.Rect(0, 0, stationMinW-2*boardPad, stationMinH-2*boardPad)

	for name, r := range inners {
		if r.Dy() == 0 {
			t.Fatalf("%s: section never laid out", name)
		}
		for n := 0; n <= 8; n++ {
			rows := boardRowRects(r, n)
			if n == 0 {
				if rows != nil {
					t.Errorf("%s n=0: %d rows, want nil", name, len(rows))
				}
				continue
			}
			if len(rows) != n {
				t.Fatalf("%s n=%d: got %d rows", name, n, len(rows))
			}
			for i, row := range rows {
				if !row.In(r) {
					t.Errorf("%s n=%d row %d = %v outside inner %v", name, n, i, row, r)
				}
				if i > 0 {
					prev := rows[i-1]
					if row.Min.Y-prev.Max.Y != boardRowGap {
						t.Errorf("%s n=%d gap %d = %d, want %d",
							name, n, i, row.Min.Y-prev.Max.Y, boardRowGap)
					}
					if row.Overlaps(prev) {
						t.Errorf("%s n=%d row %d overlaps %v", name, n, i, prev)
					}
				}
				if row.Dy() != rows[0].Dy() {
					t.Errorf("%s n=%d row %d height %d, want %d (equal rows)",
						name, n, i, row.Dy(), rows[0].Dy())
				}
			}
		}
	}
}

// sectionName labels the pumped geometry for failure messages.
func sectionName(size [2]int) string {
	switch size[0] {
	case 1280:
		return "1280×720"
	default:
		return "1024×700"
	}
}

// ---------------------------------------------------------------------------
// Geometry: the window never overlaps the log
// ---------------------------------------------------------------------------

// TestStationWindowGeometry pumps the docked board at both design sizes —
// with the worst-case row set — and pins the window: on screen, above the
// log, and every row inside the window rect at the rects boardRowRects
// produced.
func TestStationWindowGeometry(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := dockedShowcaseGame(t, size[0], size[1])

		// Worst case: every contract accepted → 3 available rows plus the
		// capped active section (1 contract + "+N more").
		setMissions(t, g,
			sim.MissionState{ID: "contract-solace-cleanup-1"},
			sim.MissionState{ID: "contract-solace-cordite-1"},
			sim.MissionState{ID: "contract-solace-supply-run-1"},
			sim.MissionState{ID: "contract-penumbra-den-hunt-1"},
			sim.MissionState{ID: "contract-penumbra-lucidite-1"},
		)
		g.syncMounts()
		pump(t, g, size[0], size[1])

		station := g.svcWin.GetContainer().GetWidget().Rect
		log := bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect

		if station.Min.X < 0 || station.Max.X > size[0] || station.Max.Y > size[1] {
			t.Errorf("%dx%d: station window %v out of bounds", size[0], size[1], station)
		}
		if station.Max.Y > log.Min.Y {
			t.Errorf("%dx%d: station window %v overlaps the log %v",
				size[0], size[1], station, log)
		}

		// Every row sits at the rect boardRowRects hands out, inside the window.
		for _, sec := range []*widget.Container{g.stAvail, g.stActive} {
			children := sec.Children()
			want := boardRowRects(sec.GetWidget().Rect, len(children))
			if len(want) != len(children) {
				t.Fatalf("%dx%d: section %d children, %d rects",
					size[0], size[1], len(children), len(want))
			}
			for i, ch := range children {
				if ch.GetWidget().Rect != want[i] {
					t.Errorf("%dx%d: row %d rect %v, want %v (boardRowRects)",
						size[0], size[1], i, ch.GetWidget().Rect, want[i])
				}
				if !ch.GetWidget().Rect.In(station) {
					t.Errorf("%dx%d: row %d %v outside the station window %v",
						size[0], size[1], i, ch.GetWidget().Rect, station)
				}
				// The row's content must fit its rect — taller or wider
				// content would bleed into the gap, the next row or the
				// window edge.
				pw, ph := ch.PreferredSize()
				if ph > ch.GetWidget().Rect.Dy() {
					t.Errorf("%dx%d: row %d content is %dpx tall, rect is %dpx",
						size[0], size[1], i, ph, ch.GetWidget().Rect.Dy())
				}
				if pw > ch.GetWidget().Rect.Dx() {
					t.Errorf("%dx%d: row %d content is %dpx wide, rect is %dpx",
						size[0], size[1], i, pw, ch.GetWidget().Rect.Dx())
				}
			}
		}
	}
}

// TestStationBoardRendersShippedContent pins the rendered row text of the
// shipped contracts: full objective/reward strings (no ellipsis — the row
// columns are wide enough) and the gated requirement on the Havstov board.
func TestStationBoardRendersShippedContent(t *testing.T) {
	g := dockedShowcaseGame(t, 1024, 700)

	if got, want := availLine(t, g, "contract-solace-cleanup-1", 1), "Destroy 1× Scuttler"; got != want {
		t.Errorf("cleanup summary = %q, want %q", got, want)
	}
	if got, want := availLine(t, g, "contract-solace-cleanup-1", 2), "40,000.00 € · +0.10 standing"; got != want {
		t.Errorf("cleanup reward = %q, want %q", got, want)
	}
	if got, want := availLine(t, g, "contract-solace-supply-run-1", 1),
		"Deliver 1 m³ 40mm ion to Havstov"; got != want {
		t.Errorf("supply summary = %q, want %q", got, want)
	}
	if got, want := availLine(t, g, "contract-solace-cordite-1", 1), "Mine 400 m³ Kerngrit Ore"; got != want {
		t.Errorf("cordite summary = %q, want %q", got, want)
	}
	for _, id := range []content.ContractID{
		"contract-solace-cleanup-1", "contract-solace-cordite-1", "contract-solace-supply-run-1",
	} {
		for line := 0; line < 3; line++ {
			if s := availLine(t, g, id, line); strings.Contains(s, "…") {
				t.Errorf("%s line %d ellipsized: %q", id, line, s)
			}
		}
	}
	// Active placeholder: nothing accepted yet.
	if got := activeTitle(t, g, 0); got != "No active contracts." {
		t.Errorf("empty active section = %q", got)
	}

	// The active line is the title + progress at the tiny step. The
	// station column is narrower since the rail grew (stationWinW =
	// 404 − stationX), so the longest shipped title now ellipsizes its
	// progress suffix rather than fitting whole — the board's content
	// floor, not a regression in the row metrics.
	setMissions(t, g, sim.MissionState{ID: "contract-solace-supply-run-1"})
	g.syncMounts()
	pump(t, g, 1024, 700)
	if got, want := activeTitle(t, g, 0), "Supply run to Havstov — …"; got != want {
		t.Errorf("supply active line = %q, want %q", got, want)
	}

	// Gated board: dock at Havstov (standing 0 < 0.20).
	g.s.Docked.Set(g.s.Player(), sim.Docked{At: "cel-penumbra-port"})
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1024, 700)
	if len(g.stAvailRows) != 2 {
		t.Fatalf("Havstov board rows = %d, want 2", len(g.stAvailRows))
	}
	for _, r := range g.stAvailRows {
		if !r.btn.GetWidget().Disabled {
			t.Errorf("%s: gated Accept button is enabled", r.id)
		}
	}
	// The requirement rides on the reward line and the reward stays whole.
	row := g.stAvail.Children()[0].(*widget.Container)
	line2 := row.Children()[1].(*widget.Container)
	if got := line2.Children()[1].(*widget.Text).Label; got != "Requires standing 0.20" {
		t.Errorf("gated requirement = %q", got)
	}
	// The reward owns line 3 on its own, so the payout never truncates.
	if got := row.Children()[2].(*widget.Text).Label; strings.Contains(got, "…") {
		t.Errorf("gated reward ellipsized: %q", got)
	}
}

// TestBoardRewardUsesCanonicalCredits: the payout line renders through
// the exported formatter every credit readout shares (theme.FormatCredits
// — HUD wallet, market title, board reward). One golden call, no
// widget-tree spelunking: with a single formatter there is nothing left
// to cross-check. The formatter's own goldens live in uikit/?/hud_test.go.
func TestBoardRewardUsesCanonicalCredits(t *testing.T) {
	if got, want := theme.FormatCredits(40000), "40,000.00 €"; got != want {
		t.Errorf("FormatCredits(40000) = %q, want %q", got, want)
	}
	def := content.ContractDef{Reward: content.ContractReward{Credits: 40000, StandingGain: 0.10}}
	if got, want := rewardText(&def), "40,000.00 € · +0.10 standing"; got != want {
		t.Errorf("rewardText = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Command emission: the UI only ever enqueues
// ---------------------------------------------------------------------------

// TestStationAcceptEmitsCommand: pressing Accept enqueues exactly
// CmdAcceptMission{ID}; a standing-gated row enqueues nothing even when
// its click event is forced past the disabled widget.
func TestStationAcceptEmitsCommand(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	var got []sim.Command
	g.enqueue = func(c sim.Command) {
		got = append(got, c)
		g.s.Queue(c) // forward: the recorder observes, the sim executes
	}

	row := findRow(t, g.stAvailRows, "contract-solace-cleanup-1")
	if row.btn.GetWidget().Disabled {
		t.Fatal("ungated Accept button is disabled")
	}
	click(t, g, row.btn)
	if len(got) != 1 {
		t.Fatalf("Accept enqueued %d commands, want 1", len(got))
	}
	cmd, ok := got[0].(sim.CmdAcceptMission)
	if !ok || cmd.ID != "contract-solace-cleanup-1" {
		t.Fatalf("Accept enqueued %#v, want CmdAcceptMission{contract-solace-cleanup-1}", got[0])
	}

	// The command actually runs: docked, so the sim accepts it.
	got = nil
	g.s.Step()
	if evs := g.s.Events(); findMissionPhase(evs, sim.MissionAccepted) == nil {
		t.Fatalf("no accepted event after Step: %v", evs)
	}
	if ms := g.s.Missions(); len(ms) != 1 || ms[0].ID != "contract-solace-cleanup-1" {
		t.Fatalf("missions = %+v, want the accepted one", ms)
	}

	// Gated rows (Havstov, standing 0): disabled and silent.
	g.s.Docked.Set(g.s.Player(), sim.Docked{At: "cel-penumbra-port"})
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	gated := findRow(t, g.stAvailRows, "contract-penumbra-den-hunt-1")
	if !gated.btn.GetWidget().Disabled {
		t.Fatal("gated Accept button is enabled")
	}
	click(t, g, gated.btn) // widget-level: the disabled check drops it
	gated.btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: gated.btn})
	if err := g.kit.Update(); err != nil {
		t.Fatalf("kit.Update: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("gated row enqueued %v, want nothing", got)
	}
}

// TestStationAbandonEmitsCommand: pressing Abandon enqueues exactly
// CmdAbandonMission{ID}.
func TestStationAbandonEmitsCommand(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	setMissions(t, g, sim.MissionState{ID: "contract-solace-cleanup-1"})
	g.syncMounts()
	pump(t, g, 1280, 720)

	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }
	row := findRow(t, g.stActRows, "contract-solace-cleanup-1")
	click(t, g, row.btn)
	if len(got) != 1 {
		t.Fatalf("Abandon enqueued %d commands, want 1", len(got))
	}
	cmd, ok := got[0].(sim.CmdAbandonMission)
	if !ok || cmd.ID != "contract-solace-cleanup-1" {
		t.Fatalf("Abandon enqueued %#v, want CmdAbandonMission{contract-solace-cleanup-1}", got[0])
	}
}

// ---------------------------------------------------------------------------
// State refresh: the EvMission path
// ---------------------------------------------------------------------------

// TestStationBoardRefreshesOnEvMission: the active row follows mission
// progress through both documented refresh paths — the per-frame board
// sync while docked (syncStationBoard runs from syncMounts) and the
// drained EvMission event, which must land without waiting for another
// frame.
func TestStationBoardRefreshesOnEvMission(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	setMissions(t, g, sim.MissionState{ID: "contract-solace-cordite-1", Progress: 0})
	g.syncMounts()
	pump(t, g, 1280, 720)
	if got, want := activeTitle(t, g, 0), "Kerngrit order — 0/400"; got != want {
		t.Fatalf("active row = %q, want %q", got, want)
	}

	// Progress changes in the sim: the next real frame's board sync
	// picks it up (the board re-syncs every frame while docked).
	setMissions(t, g, sim.MissionState{ID: "contract-solace-cordite-1", Progress: 120})
	pump(t, g, 1280, 720)
	if got, want := activeTitle(t, g, 0), "Kerngrit order — 120/400"; got != want {
		t.Fatalf("row after a frame = %q, want %q", got, want)
	}

	// The event path refreshes synchronously: no frame in between, the
	// row must show the new progress right after the event drains.
	setMissions(t, g, sim.MissionState{ID: "contract-solace-cordite-1", Progress: 240})
	if got := activeTitle(t, g, 0); got != "Kerngrit order — 120/400" {
		t.Fatalf("row changed before the event (fixture): %q", got)
	}
	g.onEvents([]sim.Event{{Kind: sim.EvMission, Phase: sim.MissionProgress}})
	if got, want := activeTitle(t, g, 0), "Kerngrit order — 240/400"; got != want {
		t.Fatalf("after EvMission: row = %q, want %q", got, want)
	}
}

// TestStationBoardRowCaps: beyond the visible caps the board collapses
// into a "+N more" row instead of growing the window.
func TestStationBoardRowCaps(t *testing.T) {
	g := dockedShowcaseGame(t, 1024, 700)
	setMissions(t, g,
		sim.MissionState{ID: "contract-solace-cleanup-1"},
		sim.MissionState{ID: "contract-solace-cordite-1"},
		sim.MissionState{ID: "contract-solace-supply-run-1"},
		sim.MissionState{ID: "contract-penumbra-den-hunt-1"},
		sim.MissionState{ID: "contract-penumbra-lucidite-1"},
	)
	g.syncMounts()
	pump(t, g, 1024, 700)

	avail := g.stAvail.Children()
	if len(avail) != availMaxRows {
		t.Fatalf("available rows = %d, want %d", len(avail), availMaxRows)
	}
	act := g.stActive.Children()
	if len(act) != actMaxRows {
		t.Fatalf("active rows = %d, want %d", len(act), actMaxRows)
	}
	// capRows(5, 2) → one contract + a note for the other four.
	first := ""
	if title := rowTitleText(act[0]); title != nil {
		first = title.Label
	}
	if !strings.Contains(first, "—") {
		t.Errorf("first active row = %q, want a contract row", first)
	}
	over := ""
	if title := rowTitleText(act[1]); title != nil {
		over = title.Label
	}
	if over != "+4 more" {
		t.Errorf("overflow row = %q, want \"+4 more\"", over)
	}
}

// findMissionPhase returns the first mission event with the given phase.
func findMissionPhase(evs []sim.Event, phase string) *sim.Event {
	for i := range evs {
		if evs[i].Kind == sim.EvMission && evs[i].Phase == phase {
			return &evs[i]
		}
	}
	return nil
}

// TestStationWindowGripStopsAtContentMin drives the contract board's
// resize grip through the real frame loop: the window can be grown by a
// bottom-edge drag, but shrinking stops at the board's own content height
// -- not at the static stationMinH the empty board used to declare -- and
// every row still sits inside the frame at that floor.
func TestStationWindowGripStopsAtContentMin(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	// Worst case: a full available board plus a capped active one, so the
	// content floor is well above the static minimum.
	setMissions(t, g, sim.MissionState{ID: "contract-solace-cleanup-1"})
	g.syncMounts()
	pump(t, g, 1280, 720)
	// Close the log: it sits directly below the board, and growing the
	// board's bottom edge into its pocket would put the shrink press on
	// the log instead of the board.
	g.host.SetOpen(idLog, false)
	g.syncMounts()
	pump(t, g, 1280, 720)

	win := g.svcWin
	start := win.GetContainer().GetWidget().Rect

	mm := useLoopMouse(t)
	pressEdge := installSeams(t, mm)
	img := ebiten.NewImage(1280, 720)
	frame := func() {
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}

	// Grow: drag the bottom edge down.
	from := image.Point{X: (start.Min.X + start.Max.X) / 2, Y: start.Max.Y - 3}
	mm.moveTo(from.X, from.Y)
	frame()
	mm.left = true
	*pressEdge = true
	frame()
	*pressEdge = false
	mm.moveTo(from.X, from.Y+200)
	frame()
	mm.left = false
	frame()
	frame()
	grown := win.GetContainer().GetWidget().Rect
	if grown.Dy() <= start.Dy() {
		t.Fatalf("bottom-edge grip did not grow the board: %v -> %v", start, grown)
	}

	// Shrink hard: the floor holds at the board's content height.
	_, ph := g.stBody.PreferredSize()
	contentH := max(ph, theme.Px(stationMinH))
	if contentH <= theme.Px(stationMinH) {
		t.Fatalf("fixture: board content height %d is not above the static %d", contentH, theme.Px(stationMinH))
	}

	from = image.Point{X: (grown.Min.X + grown.Max.X) / 2, Y: grown.Max.Y - 3}
	mm.moveTo(from.X, from.Y)
	frame()
	mm.left = true
	*pressEdge = true
	frame()
	*pressEdge = false
	mm.moveTo(from.X, from.Y-10000)
	frame()
	mm.left = false
	frame()
	frame()

	final := win.GetContainer().GetWidget().Rect
	if final.Dy() != contentH {
		t.Errorf("shrink stopped at height %d, want the board content height %d", final.Dy(), contentH)
	}
	if final.Max.Y > grown.Max.Y {
		t.Errorf("shrink grew the window: %v -> %v", grown, final)
	}

	// Pin (b): every board row sits inside the frame at the floor.
	for _, sec := range []*widget.Container{g.stAvail, g.stActive} {
		for i, ch := range sec.Children() {
			if !ch.GetWidget().Rect.In(final) {
				t.Errorf("row %d %v escapes the board frame %v", i, ch.GetWidget().Rect, final)
			}
		}
	}
}
