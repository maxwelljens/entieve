package screens

import (
	"image"
	"os"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestMapViewFitsTheViewport pins the layer contract: the rect IS the
// viewport, the canvas follows it, and a viewport change re-fits both.
func TestMapViewFitsTheViewport(t *testing.T) {
	env, _ := newChromeEnv(t)
	vp := image.Rect(0, 0, 1024, 700)
	m := NewMapView(env, func() image.Rectangle { return vp }, nil)
	m.Sync()

	// The two layers differ by data, not by panel: the map is the
	// over-chrome layer (the Screen row's OverChrome mounts it through
	// AddFrontWindow), so it must NOT answer Backdrop() and fall into the
	// under-the-rail path the proficiency tree keeps.
	if b, ok := any(m).(interface{ Backdrop() bool }); ok && b.Backdrop() {
		t.Error("the starmap still answers Backdrop(): the host would mount it under the rail")
	}
	if m.win.TitleBar != nil {
		t.Error("the layer grew a title bar")
	}
	if got := m.win.GetContainer().GetWidget().Rect; got != vp {
		t.Fatalf("layer rect = %v, want the viewport %v", got, vp)
	}
	if got := m.img.Bounds(); got != image.Rect(0, 0, 1024, 700) {
		t.Fatalf("canvas = %v, want the viewport size", got)
	}
	if m.graph.Empty() {
		t.Fatal("the graph column was not laid out")
	}

	vp = image.Rect(0, 0, 1280, 720)
	m.Sync()
	if got := m.win.GetContainer().GetWidget().Rect; got != vp {
		t.Errorf("after a viewport change the layer rect = %v, want %v", got, vp)
	}
	if got := m.img.Bounds(); got != image.Rect(0, 0, 1280, 720) {
		t.Errorf("after a viewport change the canvas = %v, want the new viewport", got)
	}
}

// fixtureSystemDef is one authored system carrying every celestial kind
// the overview knows, so no branch goes unread.
func fixtureSystemDef() *content.SystemDef {
	return &content.SystemDef{
		ID:       "sys-fixture",
		Name:     "Fixture",
		Security: "low",
		Region:   "Far Moravia",
		Celestials: []content.CelestialDef{
			{Name: "Fixture", Kind: content.CelStar},
			{Name: "Fixture Prime", Kind: content.CelPlanet},
			{Name: "Fixture Moon", Kind: content.CelMoon},
			{Name: "Fixture Rest", Kind: content.CelStation, Station: &content.StationDef{
				Services: []string{content.SvcMarket, content.SvcRefine, content.SvcIndustry},
			}},
			{Name: "Belt I", Kind: content.CelBelt, Belt: &content.BeltDef{Asteroids: []content.AsteroidDef{
				{Ore: "ore-cordite", Count: 30, Volume: 12000},
				{Ore: "ore-lucidite", Count: 18, Volume: 21000},
			}}},
			{Name: "Gate (Struvok)", Kind: content.CelGate, Gate: &content.GateDef{ToSystem: "sys-solace"}},
			{Name: "Gnazvor", Kind: content.CelAnomaly, Anomaly: &content.AnomalyDef{}},
		},
	}
}

// TestOverviewBodies pins the overview's content model: every kind appears
// in the canonical order with its manifest icon, a station carries exactly
// its services, a belt exactly its ore types with counts and volumes, and
// a gate its destination system.
func TestOverviewBodies(t *testing.T) {
	def := fixtureSystemDef()
	bodies := overviewBodies(def)
	wantKinds := []string{
		content.CelStar, content.CelPlanet, content.CelMoon, content.CelStation,
		content.CelBelt, content.CelGate, content.CelAnomaly,
	}
	wantIcons := []string{"kind_star", "kind_planet", "kind_moon", "kind_station", "kind_belt", "kind_gate", "kind_anomaly"}
	if len(bodies) != len(wantKinds) {
		t.Fatalf("bodies = %d, want %d", len(bodies), len(wantKinds))
	}
	for i, b := range bodies {
		if b.Kind != wantKinds[i] {
			t.Errorf("body %d kind = %q, want %q", i, b.Kind, wantKinds[i])
		}
		if b.Icon != wantIcons[i] {
			t.Errorf("body %d (%s) icon = %q, want %q", i, b.Name, b.Icon, wantIcons[i])
		}
	}
	// Station: exactly its services, in order.
	if got, want := bodies[3].Services, []string{content.SvcMarket, content.SvcRefine, content.SvcIndustry}; !equalStrings(got, want) {
		t.Errorf("station services = %v, want exactly %v", got, want)
	}
	// Belt: exactly its ore types with the belt's counts and volumes.
	wantOres := []overviewOre{
		{Ore: "ore-cordite", Count: 30, Volume: 12000},
		{Ore: "ore-lucidite", Count: 18, Volume: 21000},
	}
	if len(bodies[4].Ores) != len(wantOres) {
		t.Fatalf("belt ores = %v, want %v", bodies[4].Ores, wantOres)
	}
	for i, o := range bodies[4].Ores {
		if o != wantOres[i] {
			t.Errorf("belt ore %d = %+v, want %+v", i, o, wantOres[i])
		}
	}
	if bodies[5].Gate != "sys-solace" {
		t.Errorf("gate destination = %q, want sys-solace", bodies[5].Gate)
	}
	st, ga, be, an := overviewCounts(bodies)
	if got, want := [4]int{st, ga, be, an}, [4]int{1, 1, 1, 1}; got != want {
		t.Errorf("counts = %v, want %v", got, want)
	}
}

// TestOverviewBodiesRealContent pins the same reads over the shipped
// systems: a station's facility row is exactly its authored services and a
// belt's chips exactly its authored ore types.
func TestOverviewBodiesRealContent(t *testing.T) {
	env, _ := newChromeEnv(t)
	for id, def := range env.Content.Systems {
		bodies := overviewBodies(def)
		if len(bodies) != len(def.Celestials) {
			t.Errorf("%s: overview lists %d bodies, want %d", id, len(bodies), len(def.Celestials))
			continue
		}
		for _, b := range bodies {
			var cel *content.CelestialDef
			for i := range def.Celestials {
				if def.Celestials[i].Name == b.Name {
					cel = &def.Celestials[i]
					break
				}
			}
			if cel == nil {
				t.Errorf("%s: overview body %q is not in content", id, b.Name)
				continue
			}
			switch b.Kind {
			case content.CelStation:
				if !equalStrings(b.Services, cel.Station.Services) || len(b.Services) == 0 {
					t.Errorf("%s/%s: services = %v, want %v", id, b.Name, b.Services, cel.Station.Services)
				}
			case content.CelBelt:
				if len(b.Ores) != len(cel.Belt.Asteroids) || len(b.Ores) == 0 {
					t.Errorf("%s/%s: ores = %v, want %d authored", id, b.Name, b.Ores, len(cel.Belt.Asteroids))
					continue
				}
				for i, a := range cel.Belt.Asteroids {
					if b.Ores[i] != (overviewOre{Ore: a.Ore, Count: a.Count, Volume: a.Volume}) {
						t.Errorf("%s/%s: ore %d = %+v, want %+v", id, b.Name, i, b.Ores[i], a)
					}
				}
			case content.CelGate:
				if b.Gate != cel.Gate.ToSystem {
					t.Errorf("%s/%s: gate = %q, want %q", id, b.Name, b.Gate, cel.Gate.ToSystem)
				}
			}
		}
	}
}

// TestMapViewSearchNarrowsTheList pins the browser filter: a query narrows
// the visible systems (diacritics folded) and clearing it restores the
// canonical id order.
func TestMapViewSearchNarrowsTheList(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env, nil, nil)
	m.Sync()
	all := m.listedNodes()
	if len(all) != len(env.Content.Systems) {
		t.Fatalf("unfiltered list = %d, want %d", len(all), len(env.Content.Systems))
	}

	m.search.SetText("glus") // Glůšan, typed without the diacritic
	m.Sync()
	got := m.listedNodes()
	if len(got) != 1 || got[0].id != "sys-glusza" {
		t.Errorf("query glus = %v, want only Glůšan", got)
	}

	m.search.SetText("")
	m.Sync()
	if got := m.listedNodes(); len(got) != len(all) || got[0].id != all[0].id {
		t.Errorf("cleared query = %d systems (first %q), want the canonical %d", len(got), got[0].id, len(all))
	}
}

// TestMapViewHasNoInstructionText pins the owner's no-hint rule: the two
// retired instructions are gone from the source, and every string the
// layer draws from a model names content rather than telling the player
// what to do.
func TestMapViewHasNoInstructionText(t *testing.T) {
	src, err := os.ReadFile("mapview.go")
	if err != nil {
		t.Fatalf("read mapview.go: %v", err)
	}
	for _, banned := range []string{
		"Click a system",
		"Click again to set destination",
	} {
		if strings.Contains(string(src), banned) {
			t.Errorf("mapview.go still carries the retired hint %q", banned)
		}
	}

	env, _ := newChromeEnv(t)
	m := NewMapView(env, nil, nil)
	m.Sync()

	var texts []string
	for _, sec := range zoneSecs {
		texts = append(texts, zoneLabel(sec))
	}
	for _, r := range warKeyRows(env.Pal(), widgets.WarView{
		Control:  map[content.SystemID]string{"sys-solace": "charter"},
		Treasury: 1,
	}) {
		texts = append(texts, r.text)
	}
	for _, def := range env.Content.Systems {
		for _, b := range overviewBodies(def) {
			texts = append(texts, b.Name, b.Kind)
			for _, sid := range b.Services {
				texts = append(texts, serviceLabel(sid))
			}
			for _, o := range b.Ores {
				texts = append(texts, m.oreName(o.Ore))
			}
			if b.Gate != "" {
				texts = append(texts, m.systemName(b.Gate))
			}
		}
		m.sel = def.ID
		for _, l := range m.readoutLines() {
			texts = append(texts, l.text)
		}
	}
	for _, s := range texts {
		lower := strings.ToLower(s)
		for _, word := range []string{"click", "hover", "drag", "again", "set destination", "double-click"} {
			if strings.Contains(lower, word) {
				t.Errorf("drawn text %q is instruction-shaped (%q)", s, word)
			}
		}
	}
}

// TestMapViewBrowserAndGateClicks pins the two new click paths: a browser
// row selects its system, and a gate chip cross-navigates to the system on
// the other side without setting a destination.
func TestMapViewBrowserAndGateClicks(t *testing.T) {
	m, _, _, env := mapUnderTest(t)

	click := func(r image.Rectangle) {
		env.in.x, env.in.y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
		env.in.edge = [3]bool{true, false, false}
		env.Pointer().Begin()
		m.Sync()
	}

	var row *mapHit
	for i := range m.rows {
		if m.rows[i].id == "sys-glusza" {
			row = &m.rows[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("fixture: no browser row for sys-glusza in %v", m.rows)
	}
	click(row.rect)
	if m.sel != "sys-glusza" || m.dest != "" {
		t.Fatalf("row click: sel = %q dest = %q, want glusza/none", m.sel, m.dest)
	}

	// The overview follows the selection: the first row's (Gloam's) gate
	// chips are drawn on the next paint, and a chip click selects its
	// destination.
	click(m.rows[0].rect)
	if m.sel != m.rows[0].id {
		t.Fatalf("second row click: sel = %q, want %q", m.sel, m.rows[0].id)
	}
	var chip *mapHit
	for i := range m.gates {
		if m.gates[i].id == "sys-glusza" {
			chip = &m.gates[i]
			break
		}
	}
	if chip == nil {
		t.Fatalf("fixture: no gate chip to sys-glusza in %v", m.gates)
	}
	want := chip.id
	click(chip.rect)
	if m.sel != want || m.dest != "" {
		t.Fatalf("gate click: sel = %q dest = %q, want %q/none", m.sel, m.dest, want)
	}
}

// TestMapViewSearchKeysWalkTheList pins the focused field's keys: arrows
// move the selection, Enter toggles the destination, and Esc clears the
// query first and only then blurs (so the next Esc reaches the ladder).
func TestMapViewSearchKeysWalkTheList(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env, nil, nil)
	m.Sync()
	m.search.Focus(true)
	if !m.SearchFocused() {
		t.Fatal("the field did not take focus")
	}

	if !m.SearchKey(ebiten.KeyArrowDown) || m.sel != m.listedNodes()[0].id {
		t.Errorf("ArrowDown: sel = %q, want the first listed system %q", m.sel, m.listedNodes()[0].id)
	}
	if !m.SearchKey(ebiten.KeyArrowUp) || m.sel != m.listedNodes()[0].id {
		t.Errorf("ArrowUp from the top: sel = %q, want it to stay on %q", m.sel, m.listedNodes()[0].id)
	}
	if !m.SearchKey(ebiten.KeyEnter) {
		t.Fatal("Enter was not claimed")
	}
	if m.dest != m.sel {
		t.Errorf("Enter: dest = %q, want the selected %q", m.dest, m.sel)
	}
	if !m.SearchKey(ebiten.KeyEnter) || m.dest != "" {
		t.Errorf("second Enter: dest = %q, want it cleared", m.dest)
	}

	m.search.SetText("glus")
	if !m.SearchKey(ebiten.KeyEscape) {
		t.Error("Esc with text was not claimed (it must clear first)")
	}
	if m.search.GetText() != "" {
		t.Errorf("query = %q after Esc, want empty", m.search.GetText())
	}
	if !m.SearchFocused() {
		t.Error("Esc blurred the field before clearing the query")
	}
	if m.SearchKey(ebiten.KeyEscape) || m.SearchFocused() {
		t.Error("a second Esc on an empty field must blur and fall through")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
