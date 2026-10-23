package screens

import (
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// The Show Info content fixtures. The content stream authors the
// description/origin data separately (the shipped JSON leaves those
// fields empty), so every fixture below registers its own def in the
// test's freshly loaded content and code against it.

func fixtureShip(t *testing.T, env *testEnv, id content.ShipID, def *content.ShipDef) *content.ShipDef {
	t.Helper()
	def.ID = id
	if def.Name == "" {
		def.Name = "Fixture Hull"
	}
	if env.Content.Ships == nil {
		env.Content.Ships = map[content.ShipID]*content.ShipDef{}
	}
	env.Content.Ships[id] = def
	return def
}

func fixtureModule(t *testing.T, env *testEnv, id content.ModuleID, def *content.ModuleDef) *content.ModuleDef {
	t.Helper()
	def.ID = id
	if def.Name == "" {
		def.Name = "Fixture Module"
	}
	if env.Content.Modules == nil {
		env.Content.Modules = map[content.ModuleID]*content.ModuleDef{}
	}
	env.Content.Modules[id] = def
	return def
}

func fixtureItem(t *testing.T, env *testEnv, id content.ItemID, def *content.ItemDef) *content.ItemDef {
	t.Helper()
	def.ID = id
	if def.Name == "" {
		def.Name = "Fixture Item"
	}
	if env.Content.Items == nil {
		env.Content.Items = map[content.ItemID]*content.ItemDef{}
	}
	env.Content.Items[id] = def
	return def
}

func fixtureNpc(t *testing.T, env *testEnv, id content.NpcID, def *content.NpcDef) *content.NpcDef {
	t.Helper()
	def.ID = id
	if def.Name == "" {
		def.Name = "Fixture Caste"
	}
	if env.Content.NPCs == nil {
		env.Content.NPCs = map[content.NpcID]*content.NpcDef{}
	}
	env.Content.NPCs[id] = def
	return def
}

// sectionOf returns the page's section with the label (nil when the def
// did not carry the fields for it).
func sectionOf(p infoPage, label string) *infoSection {
	for i := range p.Sections {
		if p.Sections[i].Label == label {
			return &p.Sections[i]
		}
	}
	return nil
}

// rowOf returns the section row with the label (ok=false when missing).
func rowOf(s *infoSection, label string) (infoRow, bool) {
	if s == nil {
		return infoRow{}, false
	}
	for _, r := range s.Rows {
		if r.Label == label {
			return r, true
		}
	}
	return infoRow{}, false
}

// mustRow fails the test when the section has no such row.
func mustRow(t *testing.T, p infoPage, section, label string) infoRow {
	t.Helper()
	r, ok := rowOf(sectionOf(p, section), label)
	if !ok {
		t.Fatalf("page has no %s/%s row (sections %v)", section, label, sectionLabels(p))
	}
	return r
}

func sectionLabels(p infoPage) []string {
	out := make([]string, 0, len(p.Sections))
	for _, s := range p.Sections {
		out = append(out, s.Label)
	}
	return out
}

// TestInfoPageHull: the hull page renders every block only where the def
// carries the fields — construction (with the origin line only when
// Origin is set), slots, survival, capacitor, propulsion, sensors and
// cargo — and a bare hull gets no empty sections at all.
func TestInfoPageHull(t *testing.T) {
	env, _ := newChromeEnv(t)

	full := fixtureShip(t, env, "ship-fixture-full", &content.ShipDef{
		Class:  "frigate",
		Origin: "moravian",
		Slots:  content.Slots{High: 4, Mid: 3, Low: 2},
		Shield: 700, Armor: 600, Structure: 800,
		Resists:        content.Resists{EM: 0.25, Thermal: 0.4, Kinetic: 0.5, Explosive: 0.1},
		Capacitor:      1200,
		Recharge:       320,
		MaxSpeed:       340,
		Agility:        3.2,
		Mass:           1.1e6,
		WarpSpeed:      4.5,
		SigRadius:      42,
		ScanResolution: 450,
		MaxTargetRange: 60000,
		MaxTargets:     7,
		Cargo:          450,
		OreHold:        150,
		Description:    "A heritage hull with a full stat sheet.",
	})
	page, ok := infoPageFor(env, widgets.ShipRef("ship-fixture-full"))
	if !ok {
		t.Fatal("full hull did not resolve")
	}
	if page.Title != full.Name || page.Icon != "row_hull" {
		t.Errorf("header = %q/%q, want %q/row_hull", page.Title, page.Icon, full.Name)
	}
	if page.Prose != full.Description {
		t.Errorf("prose = %q, want the description", page.Prose)
	}
	if page.Sub != "Frigate · Moravian" {
		t.Errorf("category/origin line = %q, want %q", page.Sub, "Frigate · Moravian")
	}
	for _, want := range []string{"CONSTRUCTION", "SLOTS", "SURVIVAL", "CAPACITOR",
		"PROPULSION", "SENSORS", "CARGO"} {
		if sectionOf(page, want) == nil {
			t.Errorf("missing section %s (have %v)", want, sectionLabels(page))
		}
	}
	if r := mustRow(t, page, "CONSTRUCTION", "Origin"); r.Value != "Moravian" {
		t.Errorf("origin row = %+v, want Moravian", r)
	}
	if r := mustRow(t, page, "SLOTS", "High"); r.Value != "4" {
		t.Errorf("high slots = %+v, want 4", r)
	}
	if r := mustRow(t, page, "SURVIVAL", "EM resist"); r.Value != "25%" {
		t.Errorf("EM resist = %+v, want 25%%", r)
	}
	if r := mustRow(t, page, "CAPACITOR", "Capacity"); r.Value != "1,200.0 GJ" {
		t.Errorf("capacity = %+v", r)
	}
	if r := mustRow(t, page, "PROPULSION", "Align time"); r.Value != "32.0 s" {
		t.Errorf("align = %+v (agility × 10)", r)
	}
	if r := mustRow(t, page, "CARGO", "Ore hold"); r.Value != "150 m³" {
		t.Errorf("ore hold = %+v", r)
	}

	// A hull without Origin: no origin row, no origin in the line.
	fixtureShip(t, env, "ship-fixture-bare", &content.ShipDef{
		Class:   "barge",
		Slots:   content.Slots{High: 2},
		Shield:  500,
		Cargo:   3000,
		Agility: 8,
		Mass:    2e7,
	})
	bare, ok := infoPageFor(env, widgets.ShipRef("ship-fixture-bare"))
	if !ok {
		t.Fatal("bare hull did not resolve")
	}
	if _, has := rowOf(sectionOf(bare, "CONSTRUCTION"), "Origin"); has {
		t.Error("a hull without Origin renders an origin line")
	}
	if strings.Contains(bare.Sub, "Origin") || strings.Contains(bare.Sub, "·") {
		t.Errorf("category line without Origin = %q, want just the class", bare.Sub)
	}
	// Sections whose fields are unset never render.
	for _, absent := range []string{"SENSORS", "CAPACITOR"} {
		if s := sectionOf(bare, absent); s != nil {
			t.Errorf("%s rendered with unset fields: %+v", absent, s.Rows)
		}
	}
	// CARGO exists (the hull carries a cargo bay) but the unset ore hold
	// never becomes a row.
	if _, has := rowOf(sectionOf(bare, "CARGO"), "Ore hold"); has {
		t.Error("zero ore hold rendered a row")
	}
}

// TestInfoPageModule: ACTIVATION leads (slot, kind, cycle, cap use), the
// shared moduleTip sections follow verbatim, CHARGES lists the ammo
// class plus every compatible charge as a link row, MARKET shows only
// when the def carries a price — and no block renders when empty.
func TestInfoPageModule(t *testing.T) {
	env, _ := newChromeEnv(t)

	fixtureModule(t, env, "mod-fixture-weapon", &content.ModuleDef{
		Slot:           content.SlotHigh,
		Kind:           content.KindWeapon,
		ActivationCost: 2,
		ActivationTime: 3,
		Price:          1_500_000,
		Weapon: &content.WeaponAttrs{
			Damage:    content.Damage{EM: 16, Thermal: 5, Kinetic: 2, Explosive: 1},
			Optimal:   12000,
			Falloff:   6000,
			Tracking:  145,
			AmmoClass: "small-autocannon",
		},
	})
	page, ok := infoPageFor(env, widgets.ModuleRef("mod-fixture-weapon"))
	if !ok {
		t.Fatal("module did not resolve")
	}
	if len(page.Sections) == 0 || page.Sections[0].Label != "ACTIVATION" {
		t.Fatalf("first section = %v, want ACTIVATION leading", sectionLabels(page))
	}
	if r := mustRow(t, page, "ACTIVATION", "Slot"); r.Value != "High" {
		t.Errorf("slot row = %+v, want High", r)
	}
	if r := mustRow(t, page, "ACTIVATION", "Kind"); r.Value != "Weapon" {
		t.Errorf("kind row = %+v, want Weapon", r)
	}
	if r := mustRow(t, page, "ACTIVATION", "Cycle"); r.Value != "3.0 s" {
		t.Errorf("cycle row = %+v", r)
	}
	// The shared stat blocks arrive through moduleSections — the same
	// builder the rack tooltip composes (tooltip_test pins it there).
	for _, want := range []string{"DAMAGE", "RANGE", "FIRING", "CHARGES", "MARKET"} {
		if sectionOf(page, want) == nil {
			t.Errorf("missing section %s (have %v)", want, sectionLabels(page))
		}
	}
	if r := mustRow(t, page, "DAMAGE", "Per shot"); r.Value != "24.0" {
		t.Errorf("per shot = %+v, want 24.0 (moduleTip's number)", r)
	}
	if r := mustRow(t, page, "MARKET", "Price"); r.Value != theme.FormatCredits(1_500_000) {
		t.Errorf("price row = %+v", r)
	}

	charges := sectionOf(page, "CHARGES")
	if charges == nil {
		t.Fatal("no CHARGES section")
	}
	if cls, _ := rowOf(charges, "Class"); cls.Value != "Small-autocannon" {
		t.Errorf("ammo class row = %+v, want Small-autocannon", cls)
	}
	// Every compatible charge is a link to a loaded item def.
	links := 0
	for _, r := range charges.Rows {
		if r.Label == "Class" {
			continue
		}
		if r.Ref == nil {
			t.Errorf("charge row %+v is not a link", r)
			continue
		}
		if r.RefOnValue {
			t.Errorf("charge row %+v links on the wrong side", r)
		}
		if !widgets.InfoRefExists(env, *r.Ref) {
			t.Errorf("charge ref %s does not resolve", r.Ref)
		}
		links++
	}
	if links == 0 {
		t.Fatal("no compatible charge rows (fixture ammo class has no items)")
	}

	// Passive module: no weapon blocks, no charges, no price → only what
	// the def carries.
	fixtureModule(t, env, "mod-fixture-passive", &content.ModuleDef{
		Slot:    content.SlotLow,
		Kind:    content.KindCapRecharger,
		Passive: &content.PassiveAttrs{Capacitor: 120},
	})
	p2, _ := infoPageFor(env, widgets.ModuleRef("mod-fixture-passive"))
	if sectionOf(p2, "CHARGES") != nil || sectionOf(p2, "MARKET") != nil || sectionOf(p2, "DAMAGE") != nil {
		t.Errorf("passive page carries weapon/market blocks: %v", sectionLabels(p2))
	}
	if r := mustRow(t, p2, "BONUSES", "Capacitor"); r.Value != "+120.0 GJ" {
		t.Errorf("capacitor bonus = %+v", r)
	}
}

// TestInfoPageItem: properties, market, the refine yields as links and
// the ammo damage profile — each only when the def carries it.
func TestInfoPageItem(t *testing.T) {
	env, _ := newChromeEnv(t)

	fixtureItem(t, env, "min-fixture-trit", &content.ItemDef{
		Name:      "Fixture Tritanium",
		Category:  content.CatMineral,
		Volume:    0.01,
		BasePrice: 5.5,
	})
	fixtureItem(t, env, "ore-fixture-vein", &content.ItemDef{
		Name:     "Fixture Vein Ore",
		Category: content.CatOre,
		Volume:   1,
		RefineYield: map[content.ItemID]float64{
			"min-fixture-trit":    0.6,
			"min-fixture-mystery": 0.4, // never loaded: renders without a link
		},
	})
	fixtureItem(t, env, "ammo-fixture-lance", &content.ItemDef{
		Name:      "Fixture Lance",
		Category:  content.CatAmmo,
		Volume:    0.001,
		AmmoClass: "small-autocannon",
		Damage:    &content.Damage{EM: 10, Thermal: 4, Kinetic: 3, Explosive: 2},
	})

	page, ok := infoPageFor(env, widgets.ItemRef("ore-fixture-vein"))
	if !ok {
		t.Fatal("ore did not resolve")
	}
	if page.Icon != "" {
		t.Errorf("item icon = %q, want text-only header (no item glyph in the set)", page.Icon)
	}
	if r := mustRow(t, page, "PROPERTIES", "Category"); r.Value != "Ore" {
		t.Errorf("category = %+v, want Ore", r)
	}
	if _, has := rowOf(sectionOf(page, "PROPERTIES"), "Volume"); !has {
		t.Error("no volume row on an item with a volume")
	}
	if sectionOf(page, "MARKET") != nil {
		t.Error("zero base price rendered a MARKET section")
	}
	y := sectionOf(page, "REFINES TO")
	if y == nil {
		t.Fatal("no REFINES TO section")
	}
	trit, _ := rowOf(y, "Fixture Tritanium")
	if trit.Ref == nil || !widgets.InfoRefExists(env, *trit.Ref) {
		t.Errorf("tritanium yield row = %+v, want a link to the loaded item", trit)
	}
	if trit.Value != "60%" {
		t.Errorf("yield = %q, want 60%%", trit.Value)
	}
	mystery, _ := rowOf(y, "min-fixture-mystery")
	if mystery.Ref != nil {
		t.Errorf("unknown refine target links to %s (want no link)", mystery.Ref)
	}

	// The ammo's damage profile renders as the tooltip's damage rows.
	ap, _ := infoPageFor(env, widgets.ItemRef("ammo-fixture-lance"))
	if r := mustRow(t, ap, "DAMAGE", "Per shot"); r.Value != "19.0" {
		t.Errorf("ammo per shot = %+v, want 19.0", r)
	}
	// A plain mineral has no damage block.
	mineral, _ := infoPageFor(env, widgets.ItemRef("min-fixture-trit"))
	if sectionOf(mineral, "DAMAGE") != nil || sectionOf(mineral, "REFINES TO") != nil {
		t.Errorf("mineral page = %v, want properties/market only", sectionLabels(mineral))
	}
	if r := mustRow(t, mineral, "MARKET", "Base price"); r.Value != theme.FormatCredits(5.5) {
		t.Errorf("base price = %+v", r)
	}
}

// TestInfoPageCaste: the dossier prose renders only when the content
// stream gave the caste one, the hull is a link, behavior and loot rows
// exist only when set — loot drops are links with chance and qty range.
func TestInfoPageCaste(t *testing.T) {
	env, _ := newChromeEnv(t)

	fixtureShip(t, env, "ship-fixture-rat", &content.ShipDef{Class: "rat", Name: "Fixture Rat Hull"})
	fixtureItem(t, env, "min-fixture-scrap", &content.ItemDef{Name: "Fixture Scrap", Category: content.CatLoot})

	verbose := fixtureNpc(t, env, "npc-fixture-talkative", &content.NpcDef{
		Name:          "Talkative Raider",
		Description:   "It monologues before it shoots. The dossier prose the content stream authors.",
		Faction:       "Scavengers",
		Ship:          "ship-fixture-rat",
		AggroRange:    40000,
		OrbitRange:    5000,
		FleeThreshold: 0.15,
		Bounty:        120000,
		Loot: []content.LootDrop{
			{Item: "min-fixture-scrap", Chance: 0.4, MinQty: 1, MaxQty: 3},
		},
	})
	page, ok := infoPageFor(env, widgets.NpcRef("npc-fixture-talkative"))
	if !ok {
		t.Fatal("caste did not resolve")
	}
	if page.Prose != verbose.Description {
		t.Errorf("prose = %q, want the caste dossier", page.Prose)
	}
	if page.Sub != "Rat" {
		t.Errorf("category line = %q, want the hull class", page.Sub)
	}
	hull := mustRow(t, page, "DOSSIER", "Hull")
	if hull.Ref == nil || hull.Ref.Kind != widgets.InfoShip || hull.Ref.ID != "ship-fixture-rat" {
		t.Errorf("hull row = %+v, want a link to the hull def", hull)
	}
	if !hull.RefOnValue || hull.Value != "Fixture Rat Hull" {
		t.Errorf("hull row links on the label side: %+v", hull)
	}
	if r := mustRow(t, page, "DOSSIER", "Bounty"); r.Value != theme.FormatCredits(120000) {
		t.Errorf("bounty row = %+v", r)
	}
	if r := mustRow(t, page, "BEHAVIOR", "Aggro range"); r.Value != "40.0 km" {
		t.Errorf("aggro row = %+v", r)
	}
	if r := mustRow(t, page, "BEHAVIOR", "Flee threshold"); r.Value != "15%" {
		t.Errorf("flee row = %+v", r)
	}
	loot := sectionOf(page, "LOOT")
	if loot == nil || len(loot.Rows) != 1 {
		t.Fatalf("loot section = %+v, want one drop row", loot)
	}
	if r := loot.Rows[0]; r.Ref == nil || r.Label != "Fixture Scrap" || r.RefOnValue {
		t.Errorf("loot row = %+v, want the item name as a label-side link", r)
	}
	if r := loot.Rows[0]; r.Value != "40% · ×1–3" {
		t.Errorf("loot value = %q, want chance and qty range", r.Value)
	}

	// A silent caste: no prose block at all (never placeholder text).
	fixtureNpc(t, env, "npc-fixture-silent", &content.NpcDef{
		Name:   "Silent Raider",
		Ship:   "ship-fixture-rat",
		Bounty: 5000,
	})
	silentPage, _ := infoPageFor(env, widgets.NpcRef("npc-fixture-silent"))
	if silentPage.Prose != "" {
		t.Errorf("empty Description rendered prose %q", silentPage.Prose)
	}
	// Its loot/behavior blocks are absent, the dossier keeps its bounty.
	for _, absent := range []string{"BEHAVIOR", "LOOT"} {
		if s := sectionOf(silentPage, absent); s != nil {
			t.Errorf("%s rendered for a caste without the fields: %+v", absent, s.Rows)
		}
	}
	if r := mustRow(t, silentPage, "DOSSIER", "Bounty"); r.Value != theme.FormatCredits(5000) {
		t.Errorf("silent bounty = %+v", r)
	}
}

// TestInfoRefResolution: row payloads and grid entities resolve to the
// def their window shows (NPC caste before its hull; asteroid → ore),
// and unknown ids resolve to nothing.
func TestInfoRefResolution(t *testing.T) {
	env, _ := newChromeEnv(t)
	s := env.Sim

	// The player resolves to its hull.
	ref, ok := env.InfoRefFor(s.Player())
	if !ok || ref.Kind != widgets.InfoShip {
		t.Fatalf("player ref = %v (%v), want a ship", ref, ok)
	}
	if def, _ := s.Ships.Get(s.Player()); def == nil || string(ref.ID) != string(def.Hull) {
		t.Errorf("player ref = %s, want hull %s", ref, def.Hull)
	}

	// Row payloads: every id kind the tables carry resolves through the
	// item → module → ship chain; slot picks resolve to the fitted
	// module and empty slots resolve to nothing.
	fixtureItem(t, env, "item-fixture-payload", &content.ItemDef{Name: "Payload"})
	if ref, ok := env.InfoRefOf(content.ItemID("item-fixture-payload")); !ok || ref.Kind != widgets.InfoItem {
		t.Errorf("ItemID ref = %v (%v)", ref, ok)
	}
	fit, _ := s.Fittings.Get(s.Player())
	if fit == nil || len(fit.High) == 0 {
		t.Fatal("fixture: player has no high slots")
	}
	if ref, ok := env.InfoRefOf(widgets.SlotPick{Ref: sim.SlotRef{Class: content.SlotHigh}, Mod: fit.High[0]}); !ok || ref.Kind != widgets.InfoModule {
		t.Errorf("fitted slot pick ref = %v (%v)", ref, ok)
	}
	if _, ok := env.InfoRefOf(widgets.SlotPick{Ref: sim.SlotRef{Class: content.SlotHigh}}); ok {
		t.Error("empty slot pick resolved to a def")
	}
	if _, ok := env.InfoRefOf(content.ItemID("no-such-item")); ok {
		t.Error("unknown id resolved to a def")
	}
	if _, ok := env.InfoRefOf("not a payload"); ok {
		t.Error("foreign payload resolved to a def")
	}
}

// TestInfoWindowShowAndRefSwap drives the window itself: Show reveals it
// with the "<Name> — Info" title and a built page (header, prose, ref
// rows), a ref row's right-click opens the one-entry Show Info menu and
// its double-click swaps the window's content in place, an unknown ref
// changes nothing, and Close hides it again.
func TestInfoWindowShowAndRefSwap(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	if w == nil {
		t.Fatal("environment has no info window")
	}
	if w.IsOpen() {
		t.Fatal("window starts open (it must appear on demand)")
	}
	ui := newWindowUI(t, env, w.Win())
	frame := func() { uitest.PumpWindow(t, ui, w.Sync, 1024, 700) }
	frame() // layout: the link rows get their rects

	// A weapon whose CHARGES section links the showcase light charges.
	fixtureModule(t, env, "mod-fixture-links", &content.ModuleDef{
		Slot:           content.SlotHigh,
		Kind:           content.KindWeapon,
		ActivationTime: 1,
		Weapon:         &content.WeaponAttrs{AmmoClass: "small-autocannon", Optimal: 1000},
	})
	env.ShowInfo(widgets.ModuleRef("mod-fixture-links"))
	frame()
	if !w.IsOpen() {
		t.Fatal("window did not reveal")
	}
	if got, want := w.TitleText(), "Fixture Module — Info"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if w.prose != nil {
		t.Error("a def without a description rendered a prose block")
	}
	if len(w.page.Children()) == 0 {
		t.Fatal("page container is empty")
	}
	if len(w.refRows) == 0 {
		t.Fatal("no link rows on a weapon with compatible charges")
	}

	// Right-click a link row: the one-entry Show Info menu opens on it.
	first := w.refRows[0]
	if first.Widget.Rect.Empty() {
		t.Fatal("link row never laid out")
	}
	click := func(rw InfoRefRow, btn ebiten.MouseButton) {
		t.Helper()
		rw.Widget.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
			Widget: rw.Widget,
			Button: btn,
		})
		ui.Update() // dispatch the deferred event
	}
	click(first, ebiten.MouseButtonRight)
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("right-click on a link row opened no menu")
	}
	if ents := m.Entries(); len(ents) != 1 || ents[0].Label != "Show Info" || !ents[0].Bold {
		t.Fatalf("link-row menu = %+v, want the bold Show Info entry", ents)
	}

	// Pressing Show Info swaps the window onto the linked charge.
	target := first.Ref
	targetDef, ok := env.Content.Item(content.ItemID(target.ID))
	if !ok {
		t.Fatalf("fixture: linked charge %s not in content", target.ID)
	}
	pressMenuEntry(t, env, "Show Info")
	if !w.IsOpen() {
		t.Fatal("window closed while swapping")
	}
	if ref, _ := w.Ref(); ref != target {
		t.Errorf("window ref = %v, want the linked %v", ref, target)
	}
	if got, want := w.TitleText(), targetDef.Name+" — Info"; got != want {
		t.Errorf("swapped title = %q, want %q", got, want)
	}

	// Double-click on a link row swaps the same way (a single click is
	// only bookkeeping).
	env.ShowInfo(widgets.ModuleRef("mod-fixture-links"))
	frame()
	if len(w.refRows) == 0 {
		t.Fatal("ref rows lost after re-Show")
	}
	row := w.refRows[0]
	click(row, ebiten.MouseButtonLeft)
	if ref, _ := w.Ref(); ref != (widgets.ModuleRef("mod-fixture-links")) {
		t.Fatalf("single click swapped the window (ref = %v) — the swap is a double-click", ref)
	}
	click(row, ebiten.MouseButtonLeft)
	if ref, _ := w.Ref(); ref != row.Ref {
		t.Errorf("double-click ref = %v, want the linked %v", ref, row.Ref)
	}

	// An unknown ref changes nothing (the window never goes hollow).
	keep, _ := w.Ref()
	if w.Show(widgets.InfoRef{Kind: widgets.InfoModule, ID: "mod-does-not-exist"}) {
		t.Error("unknown ref resolved")
	}
	if ref, _ := w.Ref(); ref != keep {
		t.Errorf("unknown ref swapped the content: %v → %v", keep, ref)
	}

	// Close: hidden again (the kit path also unmounts — kit_test).
	w.Close()
	if w.IsOpen() {
		t.Error("window stayed visible after Close")
	}
}

// TestInfoWindowModuleHeaderIcon: a module page leads with the module's own
// icon (its art when the content names one) at the 24 px title box; other
// pages keep the 20 px tooltip box.
func TestInfoWindowModuleHeaderIcon(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	if w == nil {
		t.Fatal("environment has no info window")
	}
	ui := newWindowUI(t, env, w.Win())
	env.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)

	line, ok := w.page.Children()[0].(*widget.Container)
	if !ok {
		t.Fatalf("page header is %T, want a container", w.page.Children()[0])
	}
	g := line.Children()[0].GetWidget().Rect
	if got := g.Dx(); got != theme.Px(theme.IconTitle) {
		t.Errorf("module header icon box = %d, want the %d px title box", got, theme.Px(theme.IconTitle))
	}
	if got := line.Children()[1].GetWidget().Rect.Min.X; got < g.Max.X+theme.Px(theme.IconGap) {
		t.Errorf("module title starts at %d, want clear of the icon %v", got, g)
	}
}

// TestInfoWindowScrollbarGutter: the Show Info page rides a reserved
// scrollbar gutter — the rail hides while a short page fits, the viewport
// stays inset either way (the gutter never collapses), and an overflowing
// page shows the handle clearing the viewport — at scale 1 and 1.5.
func TestInfoWindowScrollbarGutter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"scale 1", 1}, {"scale 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			uitest.UseScale(t, tc.scale)
			env, _ := newChromeEnv(t)
			w := env.InfoWindow()
			if w == nil {
				t.Fatal("environment has no info window")
			}
			ui := newWindowUI(t, env, w.Win())
			frame := func() { uitest.PumpWindow(t, ui, w.Sync, 1024, 700) }
			frame()

			fixtureItem(t, env, "itm-fixture-short", &content.ItemDef{Name: "Short", Description: "One line."})
			if !w.Show(widgets.ItemRef("itm-fixture-short")) {
				t.Fatal("short ref did not resolve")
			}
			frame() // layout the revealed page
			frame() // Sync sees the laid-out overflow

			body := w.rail.Container().GetWidget().Rect
			if got := body.Max.X - w.scroll.ViewRect().Max.X; got != theme.Px(theme.ScrollGutter) {
				t.Errorf("reserved gutter = %d, want ScrollGutter %d", got, theme.Px(theme.ScrollGutter))
			}
			if w.rail.Bar().GetWidget().IsVisible() {
				t.Error("rail visible over a short page")
			}

			// A long description overflows the window: the rail shows and
			// the handle clears the viewport.
			long := strings.Repeat("The dossier prose runs long. ", 200)
			fixtureItem(t, env, "itm-fixture-long", &content.ItemDef{Name: "Long", Description: long})
			if !w.Show(widgets.ItemRef("itm-fixture-long")) {
				t.Fatal("long ref did not resolve")
			}
			frame()
			frame()
			if !w.rail.Bar().GetWidget().IsVisible() {
				t.Fatalf("rail hidden over an overflowing page")
			}
			bar := w.rail.Bar().GetWidget().Rect
			if bar.Overlaps(w.scroll.ViewRect()) {
				t.Errorf("scrollbar %v intersects the viewport %v", bar, w.scroll.ViewRect())
			}
		})
	}
}

// TestInfoWindowProseWraps: the description paragraph word-wraps to the
// content column and keeps wrapping when the window narrows — no line of
// the text overruns the column at either width, the page never grows wider
// than the scroll viewport, and every block under the prose stays inside
// the page. (widget.Text wraps at MaxWidth, which Sync refits to the
// viewport; a page body pinned by its own long line used to clip itself.)
func TestInfoWindowProseWraps(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())
	frame := func() { uitest.PumpWindow(t, ui, w.Sync, 1280, 720) }
	for i := 0; i < 2; i++ {
		frame()
	}

	// One wordy paragraph, no hard breaks: it must break at spaces.
	wordy := strings.Repeat("The dossier prose runs long with ordinary words and no breaks at all. ", 12)
	fixtureItem(t, env, "itm-fixture-wordy", &content.ItemDef{
		Name: "Wordy", Description: wordy, BasePrice: 1000,
	})
	if !w.Show(widgets.ItemRef("itm-fixture-wordy")) {
		t.Fatal("wordy ref did not resolve")
	}
	for i := 0; i < 3; i++ {
		frame()
	}

	check := func(tag string) {
		t.Helper()
		if w.prose == nil {
			t.Fatalf("%s: no prose block", tag)
		}
		inner := float64(w.scroll.ViewRect().Dx() - 2*theme.Px(theme.ContentsPad))
		if inner <= 0 {
			t.Fatalf("%s: no content width (viewport %v)", tag, w.scroll.ViewRect())
		}
		if w.prose.MaxWidth != inner {
			t.Errorf("%s: prose wrap width = %v, want the content column %v", tag, w.prose.MaxWidth, inner)
		}
		if pw, _ := w.prose.PreferredSize(); float64(pw) > inner {
			t.Errorf("%s: longest prose line = %d px, want ≤ %v (the content column)", tag, pw, inner)
		}
		page, view := w.page.GetWidget().Rect, w.scroll.ViewRect()
		if page.Min.X < view.Min.X || page.Max.X > view.Max.X {
			t.Errorf("%s: page %v sticks out of the viewport %v", tag, page, view)
		}
		left, right := page.Min.X+theme.Px(theme.ContentsPad), page.Max.X-theme.Px(theme.ContentsPad)
		for i, ch := range w.page.Children() {
			if r := ch.GetWidget().Rect; r.Min.X < left || r.Max.X > right {
				t.Errorf("%s: page block %d %v leaves the content column %d..%d", tag, i, r, left, right)
			}
		}
	}
	check("default width")
	wide := w.prose.MaxWidth

	// A page swap at the same width re-wraps too: a fresh Text starts at
	// the default width, so the refit must re-apply rather than notice a
	// change.
	fixtureItem(t, env, "itm-fixture-wordy2", &content.ItemDef{
		Name: "Wordy II", Description: strings.Repeat("Another paragraph with words between its breaks. ", 12),
	})
	if !w.Show(widgets.ItemRef("itm-fixture-wordy2")) {
		t.Fatal("second wordy ref did not resolve")
	}
	for i := 0; i < 2; i++ {
		frame()
	}
	check("page swap")

	// Narrow the window: the paragraph re-wraps to the smaller column and
	// the following blocks reflow inside it.
	win := w.Win().GetContainer().GetWidget().Rect
	win.Max.X = win.Min.X + 340
	w.Win().SetLocation(win)
	for i := 0; i < 3; i++ {
		frame()
	}
	check("narrowed")
	if w.prose.MaxWidth >= wide || w.prose.MaxWidth > float64(w.scroll.ViewRect().Dx()) {
		t.Errorf("narrowed wrap width = %v, want under %v and the viewport %d",
			w.prose.MaxWidth, wide, w.scroll.ViewRect().Dx())
	}
}
