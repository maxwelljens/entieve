package screens

import (
	"image"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The station board panel runs on the seams alone: a stub ContentView
// carries the docked station read (widgets.Contracts) and a recording
// sink collects every Accept/Abandon — no sim, no game, no GPU.

// boardContent is the fixture board the stub tests render: six contracts
// at one station, one of them standing-gated.
func boardContent() *content.Content {
	def := func(id content.ContractID, minStanding float64) *content.ContractDef {
		return &content.ContractDef{
			ID: id, Title: strings.TrimPrefix(string(id), "ct-"),
			Kind: content.ContractMine, Station: "cel-stub", MinStanding: minStanding,
			Objective: content.ContractObjective{Item: "item-stub-ore", Count: 10},
			Reward:    content.ContractReward{Credits: 1000, StandingGain: 0.1},
		}
	}
	return &content.Content{
		Items: map[content.ItemID]*content.ItemDef{
			"item-stub-ore": {Name: "Stub Ore", Category: content.CatOre},
		},
		Contracts: map[content.ContractID]*content.ContractDef{
			"ct-a-open":  def("ct-a-open", 0),
			"ct-b-open":  def("ct-b-open", 0),
			"ct-c-gated": def("ct-c-gated", 0.5),
			"ct-d-spare": def("ct-d-spare", 0),
			"ct-e-spare": def("ct-e-spare", 0),
			"ct-f-spare": def("ct-f-spare", 0),
		},
	}
}

// boardPanel builds the panel over a stub view and mounts its window on a
// bare UI (so the deferred button handlers dispatch).
func boardPanel(t *testing.T, cv *stubView) (*StationBoard, *recordingSink, *ebitenui.UI) {
	t.Helper()
	rec := &recordingSink{}
	env := widgets.NewSeams(theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale()), cv, rec, nil)
	b := NewStationBoard(env)
	return b, rec, newWindowUI(t, env, b.Win())
}

// pumpBoard runs one Sync/layout frame of a mounted panel.
func pumpBoard(t *testing.T, ui *ebitenui.UI, b *StationBoard) {
	t.Helper()
	uitest.PumpWindow(t, ui, b.Sync, 1280, 720)
}

// pressButton clicks a row's button the way ebitenui dispatches it: fire
// the click event, then let the UI flush the deferred handler.
func pressButton(t *testing.T, ui *ebitenui.UI, btn *widget.Button) {
	t.Helper()
	btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: btn})
	ui.Update()
}

// findRow returns the board row with the given contract id.
func findRow(t *testing.T, rows []BoardRow, id content.ContractID) BoardRow {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no board row for %s", id)
	return BoardRow{}
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
func availLine(t *testing.T, b *StationBoard, id content.ContractID, line int) string {
	t.Helper()
	var row *widget.Container
	for _, r := range b.availRows {
		if r.ID == id {
			row = r.Root
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
func activeTitle(t *testing.T, b *StationBoard, i int) string {
	t.Helper()
	kids := b.active.Children()
	if i >= len(kids) {
		t.Fatalf("only %d active rows", len(kids))
	}
	title := rowTitleText(kids[i])
	if title == nil {
		t.Fatalf("active row %d: no title text", i)
	}
	return title.Label
}

// TestStationBoardOnStubView: the panel builds, syncs and emits from the
// stub view alone — the capped board renders its visible rows and a
// "+N more" note, a standing-gated row carries "Requires standing"
// beside a disabled Accept, an accepted contract wears "Accepted", and
// Accept/Abandon reach the recording sink as typed intents. FitSize
// answers the rebuilt content floor (above the static StationMinH).
func TestStationBoardOnStubView(t *testing.T) {
	c := boardContent()
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{
		Station: "cel-stub",
		Board:   c.Board("cel-stub"),
		Active: []sim.MissionState{
			{ID: "ct-a-open", Progress: 3},
			{ID: "ct-c-gated"}, {ID: "ct-d-spare"},
		},
	}
	b, rec, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)

	// Capped: 6 offerings → two rows and a "+4 more" note; 3 accepted →
	// one row and a "+2 more" note (capRows(3, 2)).
	if got := len(b.avail.Children()); got != availMaxRows {
		t.Fatalf("available rows = %d, want %d", got, availMaxRows)
	}
	if got := len(b.actRows); got != 2 {
		t.Fatalf("active rows = %d, want the accepted row + its note", got)
	}
	if got := availLine(t, b, "ct-a-open", 0); got != "a-open" {
		t.Errorf("accepted row title = %q", got)
	}
	if got := findRow(t, b.availRows, "ct-a-open").Btn.Text().Label; got != "Accepted" {
		t.Errorf("accepted row button = %q, want Accepted", got)
	}
	if !findRow(t, b.availRows, "ct-a-open").Btn.GetWidget().Disabled {
		t.Error("accepted row's button is enabled")
	}
	note := rowTitleText(b.avail.Children()[2])
	if note == nil || note.Label != "+4 more" {
		t.Errorf("available overflow note = %v, want \"+4 more\"", note)
	}
	if got := activeTitle(t, b, 1); got != "+2 more" {
		t.Errorf("active overflow note = %q, want \"+2 more\"", got)
	}

	// FitSize is the rebuilt content floor: taller than the static
	// minimum, at the column's fixed width.
	w, h := b.FitSize()
	if w != StationWinW {
		t.Errorf("FitSize width = %d, want the column %d", w, StationWinW)
	}
	if h <= StationMinH {
		t.Errorf("FitSize height = %d, want above the static %d", h, StationMinH)
	}

	// The ungated row is the one live verb: press it (Accept).
	open := findRow(t, b.availRows, "ct-b-open")
	if open.Btn.GetWidget().Disabled {
		t.Fatal("ungated Accept button is disabled")
	}
	if got := open.Btn.Text().Label; got != "Accept" {
		t.Errorf("open row button = %q, want Accept", got)
	}
	pressButton(t, ui, open.Btn)
	if len(rec.got) != 1 {
		t.Fatalf("Accept emitted %#v, want one intent", rec.got)
	}
	if a, ok := rec.got[0].(widgets.Accept); !ok || a.Contract != "ct-b-open" {
		t.Fatalf("intent = %#v, want Accept{ct-b-open}", rec.got[0])
	}

	// Abandon rides the accepted row.
	pressButton(t, ui, findRow(t, b.actRows, "ct-a-open").Btn)
	if len(rec.got) != 2 {
		t.Fatalf("Abandon emitted %#v, want a second intent", rec.got)
	}
	if a, ok := rec.got[1].(widgets.Abandon); !ok || a.Contract != "ct-a-open" {
		t.Fatalf("intent = %#v, want Abandon{ct-a-open}", rec.got[1])
	}

	// The gated board (three offerings, all visible): the requirement
	// rides line 2 beside the summary and Accept is grayed and silent.
	cv.contracts.Board = []*content.ContractDef{
		c.Contracts["ct-a-open"], c.Contracts["ct-b-open"], c.Contracts["ct-c-gated"],
	}
	cv.contracts.Active = []sim.MissionState{{ID: "ct-a-open"}}
	pumpBoard(t, ui, b)
	if got := len(b.avail.Children()); got != 3 {
		t.Fatalf("gated board rows = %d, want 3", got)
	}
	gated := findRow(t, b.availRows, "ct-c-gated")
	if !gated.Btn.GetWidget().Disabled {
		t.Fatal("gated Accept button is enabled")
	}
	line2 := gated.Root.Children()[1].(*widget.Container)
	if got := line2.Children()[1].(*widget.Text).Label; got != "Requires standing 0.50" {
		t.Errorf("gated requirement = %q, want \"Requires standing 0.50\"", got)
	}
	before := len(rec.got)
	gated.Btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: gated.Btn})
	ui.Update()
	if len(rec.got) != before {
		t.Fatalf("gated row emitted %#v, want nothing", rec.got[before:])
	}
	// The reward owns line 3, so the payout never truncates.
	if got := availLine(t, b, "ct-c-gated", 2); strings.Contains(got, "…") {
		t.Errorf("gated reward ellipsized: %q", got)
	}
}

// TestBoardRowRects pins the pure board row layout: exact rects for a
// known inner, then the invariants (equal heights, equal gaps, pairwise
// non-overlap, everything inside the inner rect) on the panel's real
// section rect and at the window minimum.
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

	// Invariants on the laid-out section and at the window minimum.
	c := boardContent()
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{Station: "cel-stub", Board: c.Board("cel-stub")}
	b, _, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)

	inners := map[string]image.Rectangle{
		"board":          b.avail.GetWidget().Rect,
		"window minimum": image.Rect(0, 0, StationMinW-2*boardPad, StationMinH-2*boardPad),
	}
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

// TestStationBoardRowCaps: beyond the visible caps the board collapses
// into a "+N more" row instead of growing the window.
func TestStationBoardRowCaps(t *testing.T) {
	c := boardContent()
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{
		Station: "cel-stub",
		Board:   c.Board("cel-stub"),
		Active: []sim.MissionState{
			{ID: "ct-a-open"}, {ID: "ct-b-open"}, {ID: "ct-c-gated"},
			{ID: "ct-d-spare"}, {ID: "ct-e-spare"},
		},
	}
	b, _, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)

	if got := len(b.avail.Children()); got != availMaxRows {
		t.Fatalf("available rows = %d, want %d", got, availMaxRows)
	}
	act := b.active.Children()
	if len(act) != actMaxRows {
		t.Fatalf("active rows = %d, want %d", len(act), actMaxRows)
	}
	// capRows(5, 2) → one contract + a note for the other four.
	if first := activeTitle(t, b, 0); !strings.Contains(first, "—") {
		t.Errorf("first active row = %q, want a contract row", first)
	}
	if got := activeTitle(t, b, 1); got != "+4 more" {
		t.Errorf("overflow row = %q, want \"+4 more\"", got)
	}
}

// TestStationWindowGeometry: the panel window hugs its content and every
// row sits inside it, at both design sizes (the panel does not read the
// viewport; the host composes its column).
func TestStationWindowGeometry(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		c := boardContent()
		cv := &stubView{c: c, docked: true}
		cv.contracts = widgets.Contracts{
			Station: "cel-stub",
			Board:   c.Board("cel-stub"),
			Active: []sim.MissionState{
				{ID: "ct-a-open"}, {ID: "ct-b-open"}, {ID: "ct-c-gated"},
				{ID: "ct-d-spare"}, {ID: "ct-e-spare"},
			},
		}
		b, _, ui := boardPanel(t, cv)
		// The first Sync builds the rows and caches the floor; the host
		// then composes the fitted column (pinned in
		// game/screenhost_fit_test.go). Mirror it, then lay out.
		uitest.PumpWindow(t, ui, b.Sync, size[0], size[1])
		w, h := b.FitSize()
		b.Win().MinSize = &image.Point{X: theme.Px(w), Y: theme.Px(h)}
		b.Win().SetLocation(StationRect(theme.Px(h)))
		uitest.PumpWindow(t, ui, b.Sync, size[0], size[1])

		win := b.Win().GetContainer().GetWidget().Rect
		if win.Dx() != theme.Px(w) || win.Dy() != theme.Px(h) {
			t.Errorf("%dx%d: window %v, want the fitted column %dx%d",
				size[0], size[1], win, theme.Px(w), theme.Px(h))
		}
		for _, sec := range []*widget.Container{b.avail, b.active} {
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
				if !ch.GetWidget().Rect.In(win) {
					t.Errorf("%dx%d: row %d %v outside the station window %v",
						size[0], size[1], i, ch.GetWidget().Rect, win)
				}
				pw, ph := ch.PreferredSize()
				if ph > ch.GetWidget().Rect.Dy() || pw > ch.GetWidget().Rect.Dx() {
					t.Errorf("%dx%d: row %d content %dx%d escapes its rect %v",
						size[0], size[1], i, pw, ph, ch.GetWidget().Rect)
				}
			}
		}
	}
}

// TestStationBoardRendersShippedContent pins the rendered row text of the
// shipped contracts: full objective/reward strings (no ellipsis — the row
// columns are wide enough) and the gated requirement on the Hauvnok board.
func TestStationBoardRendersShippedContent(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{
		Station: "cel-haven-rest",
		Board:   c.Board("cel-haven-rest"),
		Active:  []sim.MissionState{{ID: "contract-solace-supply-run-1"}},
	}
	b, _, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)

	if got, want := availLine(t, b, "contract-solace-cordite-1", 2), "38,400.00 € · +0.10 standing"; got != want {
		t.Errorf("cordite reward = %q, want %q", got, want)
	}
	if got, want := availLine(t, b, "contract-solace-supply-run-1", 1),
		"Deliver 1 m³ 40mm ion to Hauvnok"; got != want {
		t.Errorf("supply summary = %q, want %q", got, want)
	}
	if got, want := availLine(t, b, "contract-solace-cordite-1", 1), "Mine 400 m³ Kerngrit Ore"; got != want {
		t.Errorf("cordite summary = %q, want %q", got, want)
	}
	for _, id := range []content.ContractID{
		"contract-solace-cordite-1", "contract-solace-supply-run-1",
	} {
		for line := 0; line < 3; line++ {
			if s := availLine(t, b, id, line); strings.Contains(s, "…") {
				t.Errorf("%s line %d ellipsized: %q", id, line, s)
			}
		}
	}

	// The active line is the title + progress at the tiny step. The
	// station column is narrower than the row's longest titles, so the
	// progress suffix ellipsizes rather than fitting whole — the board's
	// content floor, not a regression in the row metrics.
	if got, want := activeTitle(t, b, 0), "Supply run to Hauvnok — …"; got != want {
		t.Errorf("supply active line = %q, want %q", got, want)
	}

	// Gated board: the Hauvnok board at standing 0 < 0.20.
	cv.contracts.Station = "cel-penumbra-port"
	cv.contracts.Board = c.Board("cel-penumbra-port")
	cv.contracts.Active = nil
	pumpBoard(t, ui, b)
	if len(b.availRows) != 2 {
		t.Fatalf("Hauvnok board rows = %d, want 2", len(b.availRows))
	}
	for _, r := range b.availRows {
		if !r.Btn.GetWidget().Disabled {
			t.Errorf("%s: gated Accept button is enabled", r.ID)
		}
	}
	row := b.avail.Children()[0].(*widget.Container)
	line2 := row.Children()[1].(*widget.Container)
	if got := line2.Children()[1].(*widget.Text).Label; got != "Requires standing 0.20" {
		t.Errorf("gated requirement = %q", got)
	}
	if got := row.Children()[2].(*widget.Text).Label; strings.Contains(got, "…") {
		t.Errorf("gated reward ellipsized: %q", got)
	}
}

// TestStationBoardRefreshesOnEvMission: the active row follows mission
// progress through the mounted panel's Sync — the one refresh path (the
// game's event drain no longer refreshes the board separately).
func TestStationBoardRefreshesOnEvMission(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{
		Station: "cel-haven-rest",
		Board:   c.Board("cel-haven-rest"),
		Active:  []sim.MissionState{{ID: "contract-solace-cordite-1"}},
	}
	b, _, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)
	if got, want := activeTitle(t, b, 0), "Kerngrit order — 0/400"; got != want {
		t.Fatalf("active row = %q, want %q", got, want)
	}

	// Progress changes in the sim: the next Sync rebuilds the row.
	cv.contracts.Active = []sim.MissionState{{ID: "contract-solace-cordite-1", Progress: 120}}
	b.Sync()
	if got, want := activeTitle(t, b, 0), "Kerngrit order — 120/400"; got != want {
		t.Fatalf("row after Sync = %q, want %q", got, want)
	}
}

// TestBoardRewardUsesCanonicalCredits: the payout line renders through
// the exported formatter every credit readout shares (theme.FormatCredits
// — HUD wallet, market title, board reward).
func TestBoardRewardUsesCanonicalCredits(t *testing.T) {
	if got, want := theme.FormatCredits(40000), "40,000.00 €"; got != want {
		t.Errorf("FormatCredits(40000) = %q, want %q", got, want)
	}
	def := content.ContractDef{Reward: content.ContractReward{Credits: 40000, StandingGain: 0.10}}
	if got, want := rewardText(&def), "40,000.00 € · +0.10 standing"; got != want {
		t.Errorf("rewardText = %q, want %q", got, want)
	}
}

// TestStationAcceptEmitsCommand: pressing Accept emits exactly
// widgets.Accept{Contract}; a standing-gated row emits nothing even when
// its click event is forced past the disabled widget.
func TestStationAcceptEmitsCommand(t *testing.T) {
	c := boardContent()
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{Station: "cel-stub", Board: []*content.ContractDef{
		c.Contracts["ct-a-open"], c.Contracts["ct-b-open"], c.Contracts["ct-c-gated"],
	}}
	b, rec, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)

	row := findRow(t, b.availRows, "ct-a-open")
	if row.Btn.GetWidget().Disabled {
		t.Fatal("ungated Accept button is disabled")
	}
	pressButton(t, ui, row.Btn)
	if len(rec.got) != 1 {
		t.Fatalf("Accept emitted %d intents, want 1", len(rec.got))
	}
	if a, ok := rec.got[0].(widgets.Accept); !ok || a.Contract != "ct-a-open" {
		t.Fatalf("intent = %#v, want Accept{ct-a-open}", rec.got[0])
	}

	// Gated row: disabled and silent, even when the event is forced.
	gated := findRow(t, b.availRows, "ct-c-gated")
	if !gated.Btn.GetWidget().Disabled {
		t.Fatal("gated Accept button is enabled")
	}
	gated.Btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: gated.Btn})
	ui.Update()
	if len(rec.got) != 1 {
		t.Fatalf("gated row emitted %#v, want nothing", rec.got[1:])
	}
}

// TestStationAbandonEmitsCommand: pressing Abandon emits exactly
// widgets.Abandon{Contract}.
func TestStationAbandonEmitsCommand(t *testing.T) {
	c := boardContent()
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{
		Station: "cel-stub",
		Board:   c.Board("cel-stub"),
		Active:  []sim.MissionState{{ID: "ct-a-open"}},
	}
	b, rec, ui := boardPanel(t, cv)
	pumpBoard(t, ui, b)

	pressButton(t, ui, findRow(t, b.actRows, "ct-a-open").Btn)
	if len(rec.got) != 1 {
		t.Fatalf("Abandon emitted %d intents, want 1", len(rec.got))
	}
	if a, ok := rec.got[0].(widgets.Abandon); !ok || a.Contract != "ct-a-open" {
		t.Fatalf("intent = %#v, want Abandon{ct-a-open}", rec.got[0])
	}
}

// TestContractRowSpecData pins the board rows' declared data (the
// RowSpec the seam derives the menu from): one action — Accept on an
// available row, Abandon on an active one — carrying the model's gate,
// no Ref (a contract has no def of its own to show), and the objective
// label's Ref pointing at the objective's def.
func TestContractRowSpecData(t *testing.T) {
	c := boardContent()
	cv := &stubView{c: c, docked: true}
	cv.contracts = widgets.Contracts{Station: "cel-stub", Board: c.Board("cel-stub")}
	b, _, _ := boardPanel(t, cv)

	avail := boardRowModel{ID: "ct-a-open"}
	spec := b.contractRowSpec(avail, false)
	if spec.Ref != nil || spec.SecRef != nil {
		t.Errorf("contract row declares refs %v/%v, want none (nothing to show)", spec.Ref, spec.SecRef)
	}
	if len(spec.Actions) != 1 || spec.Actions[0].Label != "Accept" || spec.Actions[0].Disabled {
		t.Fatalf("ungated available row = %+v, want one enabled Accept", spec.Actions)
	}
	gated := boardRowModel{ID: "ct-c-gated", Disabled: true}
	if a := b.contractRowSpec(gated, false).Actions[0]; !a.Disabled {
		t.Error("standing-gated row's Accept is not grayed")
	}
	active := b.contractRowSpec(avail, true)
	if len(active.Actions) != 1 || active.Actions[0].Label != "Abandon" || active.Actions[0].Disabled {
		t.Errorf("active row = %+v, want one enabled Abandon", active.Actions)
	}

	// The objective label (line 2) declares the objective's def as its
	// Ref — Show Info, strictly, through the installer.
	ref, ok := b.objectiveRef("ct-a-open")
	if !ok || !ref.Valid() {
		t.Fatalf("objective ref = %v (%v), want a valid ref", ref, ok)
	}
}
