package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// newWarEnv builds the chrome fixture over world-default — the one world
// that carries the war (world-showcase deliberately does not).
func newWarEnv(t *testing.T) *testEnv {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-default")
	if !ok {
		t.Fatal("world-default missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	sess := NewEnv(c, s, func(sim.Command) {}, nil)
	return &testEnv{Session: sess, Sim: s, Content: c}
}

// warEnv is a Session whose War seam a test scripts: everything else is the
// real fixture view, so the map runs its production read path.
type warEnv struct {
	widgets.Seams
	war widgets.WarView
}

func (e *warEnv) War() widgets.WarView { return e.war }

// TestWarSeam pins the read seam over the real war world: every system
// carries a side seeded from authored security, the four gate-adjacent
// fronts carry meters, the treasury is the sim's, and a world without a
// war reads the zero value.
func TestWarSeam(t *testing.T) {
	env := newWarEnv(t)
	w := env.War()
	if !w.Has() {
		t.Fatal("world-default War seam is empty")
	}
	if len(w.Control) != len(env.Content.Systems) {
		t.Fatalf("control covers %d systems, want %d", len(w.Control), len(env.Content.Systems))
	}
	for id, def := range env.Content.Systems {
		want := "charter"
		if def.Security == "null" {
			want = "works"
		}
		if got := w.Control[id]; got != want {
			t.Errorf("control[%s] = %q, want %q (the authored tier)", id, got, want)
		}
	}
	if !w.WorksHeld("sys-gloam") || w.WorksHeld("sys-solace") {
		t.Errorf("WorksHeld: glusza's neighbour gloam held=%v, solace held=%v", w.WorksHeld("sys-gloam"), w.WorksHeld("sys-solace"))
	}
	if got, want := w.Treasury, env.Sim.WarTreasury(); got != want {
		t.Errorf("treasury = %v, want the sim's %v", got, want)
	}
	// The tick-0 front: every charter system gated to a works system and
	// every works system gated to a charter one.
	front := map[content.SystemID]bool{
		"sys-penumbra": true, "sys-kamieniec": true, // Kvonice, Kamžal
		"sys-gloam": true, "sys-glusza": true, // Pušťov, Glůšan
	}
	if len(w.Contest) != len(front) {
		t.Errorf("contest covers %d systems, want the 4 fronts: %v", len(w.Contest), w.Contest)
	}
	for id := range front {
		if _, ok := w.Contest[id]; !ok {
			t.Errorf("front %s carries no meter", id)
		}
	}

	// The seam copies: writing the read never reaches the simulation.
	w.Control["sys-solace"] = "works"
	if env.Sim.War().Control["sys-solace"] != "charter" {
		t.Error("War seam handed out the live control map")
	}

	plain, _ := newChromeEnv(t)
	if plain.War().Has() {
		t.Error("world-showcase War seam carries a war")
	}
}

// cardTexts renders a card model's rows for assertions.
func cardTexts(lines []mapCardLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.text
	}
	return out
}

func hasCardLine(lines []mapCardLine, text string) bool {
	for _, l := range lines {
		if l.text == text {
			return true
		}
	}
	return false
}

// TestMapWarCard pins the selected system's war readout: the holder on
// every system, the meter on a front from the holder's side, and nothing
// at all in a world without a war.
func TestMapWarCard(t *testing.T) {
	env, _ := newChromeEnv(t)
	we := &warEnv{Seams: env, war: widgets.WarView{
		Control: map[content.SystemID]string{
			"sys-solace": "charter", "sys-penumbra": "charter",
			"sys-glusza": "works",
		},
		Contest: map[content.SystemID]widgets.ContestView{
			"sys-penumbra": {Meter: 0.42, Threshold: 1},
			"sys-glusza":   {Meter: 0.17, Threshold: 1},
		},
		Treasury: 1_000_000,
	}}
	m := NewMapView(we)
	m.Sync()

	m.sel = "sys-penumbra" // charter-held front: the Works press
	lines := m.cardLines("sys-solace")
	if !hasCardLine(lines, "Held — the charter") {
		t.Errorf("charter card lacks the holder line: %q", cardTexts(lines))
	}
	if !hasCardLine(lines, "Contested — the Works press at 42%") {
		t.Errorf("charter front lacks its meter line: %q", cardTexts(lines))
	}

	m.sel = "sys-glusza" // works-held front: the charter pushes
	lines = m.cardLines("sys-solace")
	if !hasCardLine(lines, "Held — the Old Works") {
		t.Errorf("works card lacks the holder line: %q", cardTexts(lines))
	}
	if !hasCardLine(lines, "Contested — the charter pushes at 17%") {
		t.Errorf("works front lacks its meter line: %q", cardTexts(lines))
	}

	m.sel = "sys-solace" // charter-held, behind the front: holder only
	lines = m.cardLines("sys-solace")
	if !hasCardLine(lines, "Held — the charter") {
		t.Errorf("home card lacks the holder line: %q", cardTexts(lines))
	}
	for _, l := range lines {
		if strings.HasPrefix(l.text, "Contested") {
			t.Errorf("non-front card carries a meter line %q", l.text)
		}
	}

	// A quiet meter (under 1%) stays off the card: the ring marks the front.
	m.sel = "sys-penumbra"
	we.war.Contest["sys-penumbra"] = widgets.ContestView{Meter: 0.004, Threshold: 1}
	m.Sync()
	for _, l := range m.cardLines("sys-solace") {
		if strings.HasPrefix(l.text, "Contested") {
			t.Errorf("a quiet meter still renders %q", l.text)
		}
	}

	// No war, no war rows.
	we.war = widgets.WarView{}
	m.Sync()
	lines = m.cardLines("sys-solace")
	for _, l := range lines {
		if strings.HasPrefix(l.text, "Held") || strings.HasPrefix(l.text, "Contested") {
			t.Errorf("war-less card carries %q", l.text)
		}
	}
}

// TestMapWarKey pins the legend: control, the contested marker and the war
// treasury — and no key in a world without a war.
func TestMapWarKey(t *testing.T) {
	env, _ := newChromeEnv(t)
	p := env.Pal()
	rows := warKeyRows(p, widgets.WarView{
		Control:  map[content.SystemID]string{"sys-solace": "charter"},
		Treasury: 96_500_000,
	})
	want := []string{
		"Charter · holds", "Old Works · holds", "Contested · the front",
		"War treasury — 96,500,000.00 €",
	}
	if len(rows) != len(want) {
		t.Fatalf("war key rows = %d, want %d: %v", len(rows), len(want), rows)
	}
	for i, r := range rows {
		if r.text != want[i] {
			t.Errorf("row %d = %q, want %q", i, r.text, want[i])
		}
	}
	if rows[2].mark != warMarkFront || rows[0].mark != warMarkCharter || rows[1].mark != warMarkWorks {
		t.Errorf("markers = %v/%v/%v, want charter/works/front", rows[0].mark, rows[1].mark, rows[2].mark)
	}
	if got := warKeyRows(p, widgets.WarView{}); got != nil {
		t.Errorf("war-less key = %v, want none", got)
	}
}

// TestMapWarRepaints pins the repaint rule for live war state: an unchanged
// war does not repaint, a moved meter, a flip or a treasury change does.
func TestMapWarRepaints(t *testing.T) {
	env, _ := newChromeEnv(t)
	we := &warEnv{Seams: env, war: widgets.WarView{
		Control: map[content.SystemID]string{"sys-solace": "charter", "sys-gloam": "works"},
		Contest: map[content.SystemID]widgets.ContestView{"sys-penumbra": {Meter: 0.1, Threshold: 1}},
	}}
	m := NewMapView(we)
	m.Sync()
	if m.redraws != 1 {
		t.Fatalf("first sync redraws = %d, want 1", m.redraws)
	}
	m.Sync()
	if m.redraws != 1 {
		t.Fatalf("unchanged war redrew (%d)", m.redraws)
	}

	we.war = widgets.WarView{
		Control: map[content.SystemID]string{"sys-solace": "charter", "sys-gloam": "works"},
		Contest: map[content.SystemID]widgets.ContestView{"sys-penumbra": {Meter: 0.2, Threshold: 1}},
	}
	m.Sync()
	if m.redraws != 2 {
		t.Errorf("a meter move did not repaint (%d)", m.redraws)
	}

	we.war.Treasury = 42
	m.Sync()
	if m.redraws != 3 {
		t.Errorf("a treasury move did not repaint (%d)", m.redraws)
	}
}
